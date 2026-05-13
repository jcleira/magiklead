// Package bootstrap encapsulates the four-row provisioning sequence
// that turns a freshly authenticated Clerk identity into a usable
// tenant: users, tenants, user_tenants, and a free subscription. The
// whole sequence runs inside one Postgres transaction so partial state
// is impossible.
//
// Two callers consume this package:
//
//   - The protected-routes middleware, which lazy-bootstraps on first
//     request when no user row exists for the JWT's Clerk ID.
//   - The Clerk webhook handler on user.created, which front-runs the
//     lazy path when the public webhook URL is reachable.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// FreePlanLimits is the lead/sequence cap applied to every fresh
// tenant. Mirrors handler.PlanLimits["free"]; duplicated here so the
// bootstrap package doesn't depend on the handler package (which
// drags in Stripe / chi).
var FreePlanLimits = struct {
	Leads     int32
	Sequences int32
}{Leads: 100, Sequences: 300}

// ErrNoEmail is returned when the caller supplies an empty email.
// The middleware translates this into HTTP 503 with a body that names
// the JWT-template configuration as the cause, so a developer who has
// forgotten to set the email claim recognises the misconfiguration on
// the first request.
var ErrNoEmail = errors.New("bootstrap: empty email — JWT template likely missing the `email` custom claim")

// Bootstrap provisions a user + tenant + role + free subscription for
// the given Clerk identity inside a single transaction. Returns the
// resulting tenant ID. Idempotent: re-calling with the same clerkID
// returns the same tenant.
//
// displayName is optional; when empty, the workspace name falls back
// to the email's local-part.
func Bootstrap(ctx context.Context, pool *pgxpool.Pool, clerkID, email, displayName string) (uuid.UUID, error) {
	if strings.TrimSpace(email) == "" {
		return uuid.Nil, ErrNoEmail
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := repository.New(tx)

	user, err := q.UpsertUserByClerkID(ctx, repository.UpsertUserByClerkIDParams{
		ClerkID: clerkID,
		Email:   email,
		Name:    pgtype.Text{String: displayName, Valid: displayName != ""},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert user: %w", err)
	}

	// Row-level lock serialises the rest of the bootstrap so two
	// simultaneous first-requests for the same clerkID can't both
	// create a tenant.
	if _, err := q.LockUserByID(ctx, user.ID); err != nil {
		return uuid.Nil, fmt.Errorf("lock user: %w", err)
	}

	// Idempotency: if the user already has a tenant, return it
	// rather than creating a second one. Matches the behaviour the
	// CLI used to provide via GetUserByClerkID short-circuit.
	tenants, err := q.ListTenantsByUser(ctx, user.ID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list tenants: %w", err)
	}
	if len(tenants) > 0 {
		if err := tx.Commit(ctx); err != nil {
			return uuid.Nil, fmt.Errorf("commit tx: %w", err)
		}
		return uuid.UUID(tenants[0].ID.Bytes), nil
	}

	workspaceName := workspaceNameFor(displayName, email)
	tenant, err := q.CreateTenant(ctx, repository.CreateTenantParams{Name: workspaceName})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create tenant: %w", err)
	}

	if err := q.CreateUserTenant(ctx, repository.CreateUserTenantParams{
		UserID:   user.ID,
		TenantID: tenant.ID,
		Role:     pgtype.Text{String: "owner", Valid: true},
	}); err != nil {
		return uuid.Nil, fmt.Errorf("link user→tenant: %w", err)
	}

	if _, err := q.CreateSubscription(ctx, repository.CreateSubscriptionParams{
		TenantID:       tenant.ID,
		Plan:           "free",
		LeadsLimit:     FreePlanLimits.Leads,
		SequencesLimit: FreePlanLimits.Sequences,
	}); err != nil {
		return uuid.Nil, fmt.Errorf("create subscription: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit tx: %w", err)
	}

	return uuid.UUID(tenant.ID.Bytes), nil
}

// workspaceNameFor returns the human-facing tenant name. When the
// caller has a display name we use it; otherwise we fall back to the
// email's local part, matching the legacy CLI's "tim's Workspace"
// shape.
func workspaceNameFor(displayName, email string) string {
	if displayName != "" {
		return displayName + "'s Workspace"
	}
	if i := strings.IndexByte(email, '@'); i > 0 {
		return email[:i] + "'s Workspace"
	}
	return email + "'s Workspace"
}

