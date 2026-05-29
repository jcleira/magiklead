//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/handler/...
//
// Covers issue #9 acceptance criterion 4: "Unit tests for the metrics
// handler against real devpod Postgres: cover empty campaign,
// partial-progress campaign (some sent, some replied, some bounced),
// fully-completed campaign." Adds a tenant-scope rejection test as a
// security invariant alongside.

func withMetricsPool(t *testing.T) *pgxpool.Pool {
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

// metricsFixture is the minimum graph the metrics handler needs to run:
// tenant + user + play + campaign. Leads + events + unsubscribes are
// layered on per test via the fixture's helpers, and torn down by the
// same t.Cleanup that drops the base rows.
type metricsFixture struct {
	t          *testing.T
	pool       *pgxpool.Pool
	tenantID   uuid.UUID
	userID     uuid.UUID
	playID     uuid.UUID
	campaignID uuid.UUID

	leadIDs   []uuid.UUID
	personIDs []uuid.UUID
	emailIDs  []uuid.UUID
	emails    []string
}

func newMetricsFixture(t *testing.T, pool *pgxpool.Pool) *metricsFixture {
	t.Helper()
	f := &metricsFixture{
		t:          t,
		pool:       pool,
		tenantID:   uuid.New(),
		userID:     uuid.New(),
		playID:     uuid.New(),
		campaignID: uuid.New(),
	}
	ctx := context.Background()
	clerkID := "user_metrics_" + uuid.NewString()
	metricsExec(t, pool, `INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		f.userID, clerkID, "owner-"+uuid.NewString()+"@example.com")
	metricsExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "Metrics Test "+uuid.NewString())
	metricsExec(t, pool, `INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		f.userID, f.tenantID)
	metricsExec(t, pool, `INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['email']::text[])`,
		f.playID, f.tenantID, "Metrics Play")
	metricsExec(t, pool, `INSERT INTO campaigns (id, tenant_id, play_id, name, sequence) VALUES ($1, $2, $3, $4, '[]'::jsonb)`,
		f.campaignID, f.tenantID, f.playID, "Metrics Campaign")

	t.Cleanup(func() {
		for _, leadID := range f.leadIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM email_events WHERE campaign_lead_id = $1`, leadID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM campaign_leads WHERE campaign_id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM campaigns WHERE id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM plays WHERE id = $1`, f.playID)
		for _, emailID := range f.emailIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM emails WHERE id = $1`, emailID)
		}
		for _, personID := range f.personIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM persons WHERE id = $1`, personID)
		}
		for _, email := range f.emails {
			_, _ = pool.Exec(ctx, `DELETE FROM unsubscribes WHERE lower(email) = lower($1)`, email)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM unsubscribes WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.userID)
	})
	return f
}

// addLead creates a person + email + active campaign_lead under the
// fixture's campaign. Returns the campaign_lead id, which is what
// addEvent and the metrics handler key off.
func (f *metricsFixture) addLead(email string) uuid.UUID {
	f.t.Helper()
	personID := uuid.New()
	emailID := uuid.New()
	leadID := uuid.New()
	metricsExec(f.t, f.pool, `INSERT INTO persons (id, canonical_name, normalized_name) VALUES ($1, $2, $3)`,
		personID, "Lead "+uuid.NewString(), "lead "+uuid.NewString())
	metricsExec(f.t, f.pool, `INSERT INTO emails (id, email, person_id, verified_at) VALUES ($1, $2, $3, NOW())`,
		emailID, email, personID)
	metricsExec(f.t, f.pool, `INSERT INTO campaign_leads (id, campaign_id, person_id, status) VALUES ($1, $2, $3, 'active')`,
		leadID, f.campaignID, personID)
	f.personIDs = append(f.personIDs, personID)
	f.emailIDs = append(f.emailIDs, emailID)
	f.leadIDs = append(f.leadIDs, leadID)
	f.emails = append(f.emails, email)
	return leadID
}

func (f *metricsFixture) addEvent(leadID uuid.UUID, eventType string, step int32) {
	f.t.Helper()
	metricsExec(f.t, f.pool, `INSERT INTO email_events (campaign_lead_id, event_type, step) VALUES ($1, $2, $3)`,
		leadID, eventType, step)
}

func (f *metricsFixture) unsubscribeTenant(email, reason string) {
	f.t.Helper()
	metricsExec(f.t, f.pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES ($1, $2, $3)
		ON CONFLICT (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(email)) DO NOTHING`,
		f.tenantID, email, reason)
}

func metricsExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// callMetrics drives the handler with a properly-populated chi context
// and tenant id. Returns the parsed body so callers assert on the JSON
// shape rather than the raw bytes.
type metricsResponse struct {
	LeadsTotal        int64                    `json:"leads_total"`
	SentTotal         int64                    `json:"sent_total"`
	SentByStep        []map[string]json.Number `json:"sent_by_step"`
	RepliedTotal      int64                    `json:"replied_total"`
	BouncedTotal      int64                    `json:"bounced_total"`
	UnsubscribedTotal int64                    `json:"unsubscribed_total"`
}

func callMetrics(t *testing.T, h *CampaignHandler, tenantID, campaignID uuid.UUID) (*httptest.ResponseRecorder, metricsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": campaignID.String()})
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)
	var resp metricsResponse
	if rr.Code == http.StatusOK {
		dec := json.NewDecoder(rr.Body)
		dec.UseNumber()
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("decode body=%q err=%v", rr.Body.String(), err)
		}
	}
	return rr, resp
}

// Tracer: the moment a campaign exists but has no leads/events, all
// five aggregations must return zero and `sent_by_step` must be an
// empty array (not null). Proves the route → tenant check → all five
// SQL queries → JSON shape end-to-end in the simplest possible state.
func TestMetrics_TracerEmptyCampaign(t *testing.T) {
	pool := withMetricsPool(t)
	f := newMetricsFixture(t, pool)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.LeadsTotal != 0 || resp.SentTotal != 0 || resp.RepliedTotal != 0 ||
		resp.BouncedTotal != 0 || resp.UnsubscribedTotal != 0 {
		t.Errorf("empty campaign returned non-zero counts: %+v", resp)
	}
	if resp.SentByStep == nil {
		t.Error("sent_by_step is null; should be [] so the frontend can iterate without a guard")
	}
	if len(resp.SentByStep) != 0 {
		t.Errorf("sent_by_step has %d entries, want 0", len(resp.SentByStep))
	}
}

// Security: a tenant that does not own this campaign must get 404,
// indistinguishable from a campaign that does not exist at all. Mirrors
// the rest of the campaign routes — the existence of cross-tenant IDs
// must never leak through the metrics endpoint.
func TestMetrics_TenantScopeRejects(t *testing.T) {
	pool := withMetricsPool(t)
	f := newMetricsFixture(t, pool)

	h := NewCampaignHandler(repository.New(pool), nil)
	other := uuid.New()
	rr, _ := callMetrics(t, h, other, f.campaignID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
}

// Partial-progress: 3 leads, mixed event states.
//   - lead A: sent step 1, sent step 2 (two sends, single lead)
//   - lead B: sent step 1, replied
//   - lead C: 2 soft bounces + 1 hard bounce (counts as 1 bounce)
//
// Expected: leads=3, sent=3, replied=1, bounced=1, sent_by_step=[{1:2},{2:1}].
func TestMetrics_PartialProgress(t *testing.T) {
	pool := withMetricsPool(t)
	f := newMetricsFixture(t, pool)

	leadA := f.addLead("a-" + uuid.NewString() + "@example.com")
	leadB := f.addLead("b-" + uuid.NewString() + "@example.com")
	leadC := f.addLead("c-" + uuid.NewString() + "@example.com")

	f.addEvent(leadA, "sent", 1)
	f.addEvent(leadA, "sent", 2)
	f.addEvent(leadB, "sent", 1)
	f.addEvent(leadB, "replied", 1)
	f.addEvent(leadC, "soft-bounce", 1)
	f.addEvent(leadC, "soft-bounce", 1)
	f.addEvent(leadC, "bounced", 1)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.LeadsTotal != 3 {
		t.Errorf("leads_total=%d want 3", resp.LeadsTotal)
	}
	if resp.SentTotal != 3 {
		t.Errorf("sent_total=%d want 3", resp.SentTotal)
	}
	if resp.RepliedTotal != 1 {
		t.Errorf("replied_total=%d want 1", resp.RepliedTotal)
	}
	if resp.BouncedTotal != 1 {
		t.Errorf("bounced_total=%d want 1 (lead with 2 soft + 1 hard counts once)", resp.BouncedTotal)
	}
	// sent_by_step: [{step_order:1, count:2}, {step_order:2, count:1}]
	if got := len(resp.SentByStep); got != 2 {
		t.Fatalf("sent_by_step len=%d want 2: %+v", got, resp.SentByStep)
	}
	expect := []struct {
		step  string
		count string
	}{
		{"1", "2"},
		{"2", "1"},
	}
	for i, want := range expect {
		got := resp.SentByStep[i]
		if got["step_order"].String() != want.step || got["count"].String() != want.count {
			t.Errorf("sent_by_step[%d]=%+v want step_order=%s count=%s", i, got, want.step, want.count)
		}
	}
}

// Unsubscribe matching: a lead whose resolved canonical email appears
// in `unsubscribes` for this tenant counts in unsubscribed_total. The
// reverse — an unsubscribe row for an email not attached to any lead
// on this campaign — does not contribute.
func TestMetrics_UnsubscribesMatchByEmail(t *testing.T) {
	pool := withMetricsPool(t)
	f := newMetricsFixture(t, pool)

	emailIn := "in-" + uuid.NewString() + "@example.com"
	emailOut := "out-" + uuid.NewString() + "@example.com"

	f.addLead(emailIn)
	f.unsubscribeTenant(emailIn, "list-unsub")
	// Unsubscribe a totally unrelated address — must not affect this
	// campaign's metrics. The fixture's cleanup wipes any unsubscribes
	// row keyed on f.emails, so we register emailOut explicitly so the
	// teardown still finds it.
	f.emails = append(f.emails, emailOut)
	f.unsubscribeTenant(emailOut, "manual")

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.UnsubscribedTotal != 1 {
		t.Errorf("unsubscribed_total=%d want 1 (only the lead's email matches)", resp.UnsubscribedTotal)
	}
}

// Fully-completed campaign: every lead has a complete event trail
// (sent and either replied or bounced) and one is unsubscribed. The
// dashboard's claim "the campaign finished" reduces to every count
// being non-zero on the same fixture.
func TestMetrics_FullyComplete(t *testing.T) {
	pool := withMetricsPool(t)
	f := newMetricsFixture(t, pool)

	emails := []string{
		"done-1-" + uuid.NewString() + "@example.com",
		"done-2-" + uuid.NewString() + "@example.com",
		"done-3-" + uuid.NewString() + "@example.com",
	}
	leads := make([]uuid.UUID, len(emails))
	for i, e := range emails {
		leads[i] = f.addLead(e)
	}
	// Every lead got step 1; the first two also got step 2.
	for _, l := range leads {
		f.addEvent(l, "sent", 1)
	}
	f.addEvent(leads[0], "sent", 2)
	f.addEvent(leads[1], "sent", 2)
	// Outcomes.
	f.addEvent(leads[0], "replied", 2)
	f.addEvent(leads[1], "bounced", 2)
	f.unsubscribeTenant(emails[2], "list-unsub")

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.LeadsTotal != 3 || resp.SentTotal != 5 || resp.RepliedTotal != 1 ||
		resp.BouncedTotal != 1 || resp.UnsubscribedTotal != 1 {
		t.Errorf("fully-completed counts wrong: %+v", resp)
	}
	if len(resp.SentByStep) != 2 {
		t.Fatalf("sent_by_step len=%d want 2: %+v", len(resp.SentByStep), resp.SentByStep)
	}
}
