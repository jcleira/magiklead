//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/handler/...
//
// Covers issue #9 acceptance criterion 2: a capacity endpoint returns
// remaining invites this week for the connected account (cap − used,
// respecting warmup), asserted against seeded linkedin_accounts
// counters via DB + API. Reuses the shared helpers (withMetricsPool,
// metricsExec, withTenant).

// capacityFixture is the minimal graph the capacity endpoint reads: a
// tenant, optionally with one connected linkedin_account whose rolling
// counters the pacer turns into "remaining invites this week."
type capacityFixture struct {
	t        *testing.T
	pool     *pgxpool.Pool
	tenantID uuid.UUID
}

func newCapacityFixture(t *testing.T, pool *pgxpool.Pool) *capacityFixture {
	t.Helper()
	f := &capacityFixture{t: t, pool: pool, tenantID: uuid.New()}
	metricsExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "LI Capacity Test "+uuid.NewString())
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
	})
	return f
}

// addAccount inserts a connected account with the given rolling
// counters. windowAgo / warmupAgo are Postgres interval literals seeded
// relative to NOW() so the handler's time.Now() reads them in the
// intended weekly window and warmup week; an empty string leaves the
// column NULL (no window / no warmup anchor).
func (f *capacityFixture) addAccount(status string, weeklyCount int, windowAgo, warmupAgo string) {
	f.t.Helper()
	metricsExec(f.t, f.pool, `INSERT INTO linkedin_accounts
		(id, tenant_id, unipile_account_id, status, weekly_invite_count, weekly_window_started_at, warmup_started_at)
		VALUES ($1, $2, $3, $4, $5,
		        CASE WHEN $6 = '' THEN NULL ELSE NOW() - $6::interval END,
		        CASE WHEN $7 = '' THEN NULL ELSE NOW() - $7::interval END)`,
		uuid.New(), f.tenantID, "acc_cap_"+uuid.NewString(), status, weeklyCount, windowAgo, warmupAgo)
}

type capacityResponse struct {
	Connected       bool   `json:"connected"`
	Status          string `json:"status"`
	WeeklyCap       int    `json:"weekly_cap"`
	WeeklyUsed      int    `json:"weekly_used"`
	WeeklyRemaining int    `json:"weekly_remaining"`
}

func callCapacity(t *testing.T, h *UnipileHandler, tenantID uuid.UUID) (*httptest.ResponseRecorder, capacityResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/linkedin/capacity", nil)
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()
	h.Capacity(rr, req)
	var resp capacityResponse
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode body=%q err=%v", rr.Body.String(), err)
		}
	}
	return rr, resp
}

// Tracer: a tenant with no connected account reports connected=false, so
// the UI prompts a connect instead of rendering a capacity bar. Proves
// route → tenant scope → ListLinkedInAccounts → JSON in the empty state.
func TestLinkedInCapacity_TracerNoAccount(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCapacityFixture(t, pool)

	h := &UnipileHandler{queries: repository.New(pool)}
	rr, resp := callCapacity(t, h, f.tenantID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.Connected {
		t.Errorf("connected=true want false for a tenant with no account: %+v", resp)
	}
}

// A fully-warmed account mid-week: the weekly ceiling is the flat 100,
// and remaining = 100 − used against the seeded counters (warmup 60 days
// ago is well past the 4-week ramp, weekly window 1 day old is current).
func TestLinkedInCapacity_WarmedAccount(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCapacityFixture(t, pool)
	f.addAccount("active", 30, "1 day", "60 days")

	h := &UnipileHandler{queries: repository.New(pool)}
	rr, resp := callCapacity(t, h, f.tenantID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !resp.Connected {
		t.Fatalf("connected=false want true: %+v", resp)
	}
	if resp.WeeklyCap != 100 || resp.WeeklyUsed != 30 || resp.WeeklyRemaining != 70 {
		t.Errorf("got cap=%d used=%d remaining=%d want 100/30/70", resp.WeeklyCap, resp.WeeklyUsed, resp.WeeklyRemaining)
	}
}

// Respecting warmup: a week-1 account's weekly throughput is throttled
// to 7×8 = 56, so remaining this week is 56 − used, not 100 − used. This
// is the "respecting warmup" half of criterion 2 — warmup just started
// and the weekly window is current.
func TestLinkedInCapacity_WarmupWeek1RespectsRamp(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCapacityFixture(t, pool)
	f.addAccount("warming", 10, "1 hour", "1 hour")

	h := &UnipileHandler{queries: repository.New(pool)}
	rr, resp := callCapacity(t, h, f.tenantID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.WeeklyCap != 56 || resp.WeeklyUsed != 10 || resp.WeeklyRemaining != 46 {
		t.Errorf("got cap=%d used=%d remaining=%d want 56/10/46 (warmup week 1 throttles the week to 7×8)", resp.WeeklyCap, resp.WeeklyUsed, resp.WeeklyRemaining)
	}
}
