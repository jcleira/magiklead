//go:build integration

package bootstrap_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/bootstrap"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// withPool dials Postgres from DATABASE_URL and returns a pool. Skips
// the test when the env var is missing, mirroring the
// `internal/storage/s3_roundtrip_test.go` pattern. Run with:
//
//	devpods exec api go test -tags=integration ./internal/bootstrap/...
func withPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// freshClerkID returns a unique clerk_id so parallel test runs don't
// collide on the users.clerk_id UNIQUE constraint.
func freshClerkID(t *testing.T) string {
	t.Helper()
	return "user_test_" + uuid.NewString()
}

// cleanupClerkID removes the four-row graph for a clerk_id, regardless
// of test outcome. Safe to call when the rows don't exist.
func cleanupClerkID(t *testing.T, pool *pgxpool.Pool, clerkID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		WITH u AS (SELECT id FROM users WHERE clerk_id = $1),
		     ut AS (DELETE FROM user_tenants WHERE user_id IN (SELECT id FROM u) RETURNING tenant_id),
		     s AS (DELETE FROM subscriptions WHERE tenant_id IN (SELECT tenant_id FROM ut))
		DELETE FROM tenants WHERE id IN (SELECT tenant_id FROM ut);
	`, clerkID)
	if err != nil {
		t.Logf("cleanup tenants for %s: %v", clerkID, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE clerk_id = $1`, clerkID); err != nil {
		t.Logf("cleanup users for %s: %v", clerkID, err)
	}
}

func TestBootstrap_HappyPath(t *testing.T) {
	pool := withPool(t)
	clerkID := freshClerkID(t)
	t.Cleanup(func() { cleanupClerkID(t, pool, clerkID) })

	ctx := context.Background()
	tenantID, err := bootstrap.Bootstrap(ctx, pool, clerkID, "tim@example.com", "Tim Apple")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if tenantID == uuid.Nil {
		t.Fatal("Bootstrap returned uuid.Nil")
	}

	q := repository.New(pool)

	user, err := q.GetUserByClerkID(ctx, clerkID)
	if err != nil {
		t.Fatalf("GetUserByClerkID: %v", err)
	}
	if user.Email != "tim@example.com" {
		t.Errorf("user.Email=%q want tim@example.com", user.Email)
	}
	if user.Name.String != "Tim Apple" || !user.Name.Valid {
		t.Errorf("user.Name=%+v want valid 'Tim Apple'", user.Name)
	}

	tenants, err := q.ListTenantsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListTenantsByUser: %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("len(tenants)=%d want 1", len(tenants))
	}
	if uuid.UUID(tenants[0].ID.Bytes) != tenantID {
		t.Errorf("returned tenantID=%s but linked tenant=%x", tenantID, tenants[0].ID.Bytes)
	}
	if tenants[0].Name != "Tim Apple's Workspace" {
		t.Errorf("tenant.Name=%q want 'Tim Apple's Workspace'", tenants[0].Name)
	}

	sub, err := q.GetSubscription(ctx, tenants[0].ID)
	if err != nil {
		t.Fatalf("GetSubscription: %v", err)
	}
	if sub.Plan != "free" {
		t.Errorf("sub.Plan=%q want free", sub.Plan)
	}
	if sub.LeadsLimit != 100 {
		t.Errorf("sub.LeadsLimit=%d want 100", sub.LeadsLimit)
	}
	if sub.SequencesLimit != 300 {
		t.Errorf("sub.SequencesLimit=%d want 300", sub.SequencesLimit)
	}
}

func TestBootstrap_IdempotentReCall(t *testing.T) {
	pool := withPool(t)
	clerkID := freshClerkID(t)
	t.Cleanup(func() { cleanupClerkID(t, pool, clerkID) })

	ctx := context.Background()

	first, err := bootstrap.Bootstrap(ctx, pool, clerkID, "tim@example.com", "Tim Apple")
	if err != nil {
		t.Fatalf("Bootstrap (first): %v", err)
	}

	// Second call with the same clerk_id but a refreshed name —
	// simulates Clerk firing the webhook after the user updated
	// their profile, or a second protected request landing while
	// the row already exists.
	second, err := bootstrap.Bootstrap(ctx, pool, clerkID, "tim@example.com", "Tim Updated")
	if err != nil {
		t.Fatalf("Bootstrap (second): %v", err)
	}
	if first != second {
		t.Errorf("re-call produced new tenantID: first=%s second=%s", first, second)
	}

	q := repository.New(pool)

	// Tenant count for the user is exactly 1 — the second call did
	// not insert a duplicate workspace.
	user, err := q.GetUserByClerkID(ctx, clerkID)
	if err != nil {
		t.Fatalf("GetUserByClerkID: %v", err)
	}
	tenants, err := q.ListTenantsByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListTenantsByUser: %v", err)
	}
	if len(tenants) != 1 {
		t.Fatalf("len(tenants)=%d want 1", len(tenants))
	}

	// Subscription count is exactly 1 too — the second call did not
	// stack a second free subscription on the same tenant.
	var subCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions WHERE tenant_id = $1`, tenants[0].ID).Scan(&subCount); err != nil {
		t.Fatalf("count subs: %v", err)
	}
	if subCount != 1 {
		t.Errorf("subscription count=%d want 1", subCount)
	}

	// User name was refreshed by the upsert — proves the same path
	// keeps Clerk-side display name in sync without us needing the
	// user.updated webhook to re-run.
	if user.Name.String != "Tim Updated" {
		t.Errorf("user.Name=%q want 'Tim Updated' (upsert should refresh)", user.Name.String)
	}
}

func TestBootstrap_ErrNoEmail(t *testing.T) {
	pool := withPool(t)
	clerkID := freshClerkID(t)
	t.Cleanup(func() { cleanupClerkID(t, pool, clerkID) })

	ctx := context.Background()
	_, err := bootstrap.Bootstrap(ctx, pool, clerkID, "", "Tim Apple")
	if !errors.Is(err, bootstrap.ErrNoEmail) {
		t.Fatalf("err=%v want ErrNoEmail", err)
	}

	// Whitespace-only is also empty for our purposes — the JWT
	// template might have an empty `{{user.primary_email_address}}`
	// expansion that yields a blank string.
	if _, err := bootstrap.Bootstrap(ctx, pool, clerkID, "   ", ""); !errors.Is(err, bootstrap.ErrNoEmail) {
		t.Fatalf("whitespace email err=%v want ErrNoEmail", err)
	}

	// And no user row was written — the early return must precede
	// the transaction, so a missing email never leaves an orphan.
	q := repository.New(pool)
	if _, err := q.GetUserByClerkID(ctx, clerkID); err == nil {
		t.Error("user row exists after ErrNoEmail return; should have been a no-op")
	}
}

func TestBootstrap_TransactionRollback(t *testing.T) {
	pool := withPool(t)
	clerkID := freshClerkID(t)
	t.Cleanup(func() { cleanupClerkID(t, pool, clerkID) })

	ctx := context.Background()

	// Constructed constraint violation: a CHECK on user_tenants that
	// rejects role='owner' fires on the third write of the bootstrap
	// sequence. The whole tx must roll back, leaving zero rows in
	// any of the four tables for this clerk_id.
	if _, err := pool.Exec(ctx, `ALTER TABLE user_tenants ADD CONSTRAINT bootstrap_test_failure CHECK (role <> 'owner') NOT VALID`); err != nil {
		t.Fatalf("install CHECK: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `ALTER TABLE user_tenants DROP CONSTRAINT IF EXISTS bootstrap_test_failure`); err != nil {
			t.Logf("drop CHECK: %v", err)
		}
	})

	if _, err := bootstrap.Bootstrap(ctx, pool, clerkID, "tim@example.com", "Tim Apple"); err == nil {
		t.Fatal("Bootstrap returned nil error; expected rollback path")
	}

	// All four tables must be clean for this clerk_id. The CHECK
	// fired on the third insert, so a non-rollback would have left
	// users + tenants behind.
	var (
		userCount, tenantLinks, tenantCount, subCount int
	)
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE clerk_id = $1`, clerkID).Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount != 0 {
		t.Errorf("users count=%d for %s, want 0 after rollback", userCount, clerkID)
	}

	// Joined counts: any orphan tenant or user_tenant or subscription
	// pointing back to this clerk_id would mean the rollback did not
	// cover that table.
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_tenants ut
		JOIN users u ON u.id = ut.user_id WHERE u.clerk_id = $1
	`, clerkID).Scan(&tenantLinks); err != nil {
		t.Fatalf("count user_tenants: %v", err)
	}
	if tenantLinks != 0 {
		t.Errorf("user_tenants count=%d, want 0", tenantLinks)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM tenants t
		JOIN user_tenants ut ON ut.tenant_id = t.id
		JOIN users u ON u.id = ut.user_id WHERE u.clerk_id = $1
	`, clerkID).Scan(&tenantCount); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if tenantCount != 0 {
		t.Errorf("tenants count=%d, want 0", tenantCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions s
		JOIN user_tenants ut ON ut.tenant_id = s.tenant_id
		JOIN users u ON u.id = ut.user_id WHERE u.clerk_id = $1
	`, clerkID).Scan(&subCount); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if subCount != 0 {
		t.Errorf("subscriptions count=%d, want 0", subCount)
	}
}

func TestBootstrap_WorkspaceNameFallback(t *testing.T) {
	pool := withPool(t)
	clerkID := freshClerkID(t)
	t.Cleanup(func() { cleanupClerkID(t, pool, clerkID) })

	ctx := context.Background()
	tenantID, err := bootstrap.Bootstrap(ctx, pool, clerkID, "tim@example.com", "")
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	q := repository.New(pool)
	tenant, err := q.GetTenant(ctx, pgUUID(tenantID))
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if tenant.Name != "tim's Workspace" {
		t.Errorf("tenant.Name=%q want 'tim's Workspace' (email local-part fallback)", tenant.Name)
	}
}
