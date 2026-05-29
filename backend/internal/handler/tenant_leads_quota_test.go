//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/handler"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// AC: saving the same person to multiple campaigns / sequences must
// count as one quota unit, not many (issue #7).
func TestTenantLead_Quota_IncrementOncePerPerson(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()

	tenantID := freshTenant(t, pool)
	userID := freshUser(t, pool, "owner-")
	personID := freshPerson(t, pool)

	if _, err := pool.Exec(ctx,
		`INSERT INTO subscriptions (tenant_id, plan, leads_limit, leads_used, sequences_limit)
		 VALUES ($1, 'free', 100, 0, 5)`, tenantID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	h := handler.NewTenantLeadHandler(repository.New(pool))
	clerkID := "clerk-" + uuid.NewString()
	if _, err := pool.Exec(ctx,
		`UPDATE users SET clerk_id = $2 WHERE id = $1`, userID, clerkID); err != nil {
		t.Fatalf("set clerk id: %v", err)
	}

	saveBody := mustJSON(t, map[string]string{"person_id": personID.String()})

	doSave := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant_leads",
			bytes.NewReader(saveBody))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
		ctx = context.WithValue(ctx, middleware.UserClerkIDKey, clerkID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.Add(rr, req)
		return rr
	}

	// First save → counter goes 0 → 1, inserted=true.
	rr := doSave()
	if rr.Code != http.StatusCreated {
		t.Fatalf("first save status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := leadsUsed(t, pool, tenantID); got != 1 {
		t.Errorf("after first save, leads_used=%d, want 1", got)
	}

	// Re-save same person → counter MUST stay at 1.
	rr = doSave()
	if rr.Code != http.StatusCreated {
		t.Fatalf("second save status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := leadsUsed(t, pool, tenantID); got != 1 {
		t.Errorf("after second save of same person, leads_used=%d, want 1", got)
	}

	// Third save → still 1.
	rr = doSave()
	if rr.Code != http.StatusCreated {
		t.Fatalf("third save status=%d", rr.Code)
	}
	if got := leadsUsed(t, pool, tenantID); got != 1 {
		t.Errorf("after third save of same person, leads_used=%d, want 1", got)
	}

	// Saving a DIFFERENT person bumps the counter to 2.
	other := freshPerson(t, pool)
	otherBody := mustJSON(t, map[string]string{"person_id": other.String()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant_leads",
		bytes.NewReader(otherBody))
	req.Header.Set("Content-Type", "application/json")
	c := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	c = context.WithValue(c, middleware.UserClerkIDKey, clerkID)
	req = req.WithContext(c)
	rr = httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("other-person save status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := leadsUsed(t, pool, tenantID); got != 2 {
		t.Errorf("after saving different person, leads_used=%d, want 2", got)
	}

	// Verify the response shape carries the lead back as JSON.
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["person_id"] != other.String() {
		t.Errorf("response person_id=%v, want %s", resp["person_id"], other)
	}
}

func freshUser(t *testing.T, pool *pgxpool.Pool, prefix string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	clerk := "clerk-" + uuid.NewString()
	email := prefix + uuid.NewString() + "@example.test"
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		id, clerk, email); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func freshPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	name := "Quota Person " + uuid.NewString()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO persons (id, canonical_name, normalized_name)
		 VALUES ($1, $2, LOWER($2))`, id, name); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	return id
}

func leadsUsed(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) int {
	t.Helper()
	var used int
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(leads_used, 0) FROM subscriptions WHERE tenant_id = $1`,
		tenantID).Scan(&used); err != nil {
		t.Fatalf("read leads_used: %v", err)
	}
	return used
}
