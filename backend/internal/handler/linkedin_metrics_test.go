//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/handler/...
//
// Covers issue #9 acceptance criterion 1: a per-campaign LinkedIn
// metrics endpoint returns counts + rates derived from `linkedin_events`,
// asserted against a seeded event set via DB + API. Mirrors the email
// metrics handler/test (campaigns_metrics_test.go) — reusing its shared
// helpers (withMetricsPool, metricsExec, withTenant, withURLParams).

// linkedInMetricsFixture is the minimum graph the LinkedIn metrics
// handler needs: tenant + user + play + campaign. Leads +
// linkedin_events are layered on per test via the fixture's helpers and
// torn down by the same t.Cleanup that drops the base rows.
type linkedInMetricsFixture struct {
	t          *testing.T
	pool       *pgxpool.Pool
	q          *repository.Queries
	tenantID   uuid.UUID
	userID     uuid.UUID
	playID     uuid.UUID
	campaignID uuid.UUID

	leadIDs    []uuid.UUID
	personIDs  []uuid.UUID
	accountIDs []uuid.UUID
}

func newLinkedInMetricsFixture(t *testing.T, pool *pgxpool.Pool) *linkedInMetricsFixture {
	t.Helper()
	f := &linkedInMetricsFixture{
		t:          t,
		pool:       pool,
		q:          repository.New(pool),
		tenantID:   uuid.New(),
		userID:     uuid.New(),
		playID:     uuid.New(),
		campaignID: uuid.New(),
	}
	clerkID := "user_li_metrics_" + uuid.NewString()
	metricsExec(t, pool, `INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		f.userID, clerkID, "owner-"+uuid.NewString()+"@example.com")
	metricsExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "LI Metrics Test "+uuid.NewString())
	metricsExec(t, pool, `INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		f.userID, f.tenantID)
	metricsExec(t, pool, `INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		f.playID, f.tenantID, "LI Metrics Play")
	metricsExec(t, pool, `INSERT INTO campaigns (id, tenant_id, play_id, name, sequence) VALUES ($1, $2, $3, $4, '[]'::jsonb)`,
		f.campaignID, f.tenantID, f.playID, "LI Metrics Campaign")

	t.Cleanup(func() {
		ctx := context.Background()
		for _, leadID := range f.leadIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, leadID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM campaign_leads WHERE campaign_id = $1`, f.campaignID)
		for _, accountID := range f.accountIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM linkedin_accounts WHERE id = $1`, accountID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM campaigns WHERE id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM plays WHERE id = $1`, f.playID)
		// The reply path (RecordReplyByPerson) writes a person-keyed
		// unsubscribes row; drop it before the persons/tenant it references.
		_, _ = pool.Exec(ctx, `DELETE FROM unsubscribes WHERE tenant_id = $1`, f.tenantID)
		for _, personID := range f.personIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM persons WHERE id = $1`, personID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.userID)
	})
	return f
}

// addLead creates a person + active LinkedIn campaign_lead under the
// fixture's campaign. Returns the campaign_lead id, which is what
// addEvent and the metrics handler key off. LinkedIn leads are
// person-keyed, so (unlike the email fixture) no email row is created.
func (f *linkedInMetricsFixture) addLead() uuid.UUID {
	f.t.Helper()
	personID := uuid.New()
	leadID := uuid.New()
	metricsExec(f.t, f.pool, `INSERT INTO persons (id, canonical_name, normalized_name) VALUES ($1, $2, $3)`,
		personID, "Lead "+uuid.NewString(), "lead "+uuid.NewString())
	metricsExec(f.t, f.pool, `INSERT INTO campaign_leads (id, campaign_id, person_id, status) VALUES ($1, $2, $3, 'active')`,
		leadID, f.campaignID, personID)
	f.personIDs = append(f.personIDs, personID)
	f.leadIDs = append(f.leadIDs, leadID)
	return leadID
}

// addAccount inserts a linkedin_accounts row for the fixture's tenant and
// returns its id. The acceptance-rate consistency test needs it because the
// pacer breaker's source, GetLinkedInAcceptanceStats, scopes events by
// campaign_leads.linkedin_account_id.
func (f *linkedInMetricsFixture) addAccount() uuid.UUID {
	f.t.Helper()
	accountID := uuid.New()
	metricsExec(f.t, f.pool, `INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		accountID, f.tenantID, "unipile_"+uuid.NewString())
	f.accountIDs = append(f.accountIDs, accountID)
	return accountID
}

// addLeadForAccount is addLead with the campaign_lead bound to a specific
// LinkedIn account, so the breaker's per-account view
// (GetLinkedInAcceptanceStats) and the dashboard's per-campaign view
// (CountCampaignLinkedInEvents) cover the same event set.
func (f *linkedInMetricsFixture) addLeadForAccount(accountID uuid.UUID) uuid.UUID {
	f.t.Helper()
	personID := uuid.New()
	leadID := uuid.New()
	metricsExec(f.t, f.pool, `INSERT INTO persons (id, canonical_name, normalized_name) VALUES ($1, $2, $3)`,
		personID, "Lead "+uuid.NewString(), "lead "+uuid.NewString())
	metricsExec(f.t, f.pool, `INSERT INTO campaign_leads (id, campaign_id, person_id, status, linkedin_account_id) VALUES ($1, $2, $3, 'active', $4)`,
		leadID, f.campaignID, personID, accountID)
	f.personIDs = append(f.personIDs, personID)
	f.leadIDs = append(f.leadIDs, leadID)
	return leadID
}

// addEvent writes one linkedin_event through the SAME repository call the
// production engine uses — CreateLinkedInEvent — rather than a raw INSERT.
// The send worker (invite_sent / dm_sent / failed), the accept reconcile
// (accepted), the webhook, and the reply path all funnel through this
// method, so events seeded here are byte-for-byte what the real flow emits;
// the metrics endpoint is therefore asserted against real events, not
// imagined ones (issue #9 criterion 3).
func (f *linkedInMetricsFixture) addEvent(leadID uuid.UUID, eventType string, step int32) {
	f.t.Helper()
	if _, err := f.q.CreateLinkedInEvent(context.Background(), repository.CreateLinkedInEventParams{
		CampaignLeadID: pgUUID(leadID),
		EventType:      eventType,
		Step:           step,
	}); err != nil {
		f.t.Fatalf("CreateLinkedInEvent(%s): %v", eventType, err)
	}
}

// linkedInMetricsResponse is the documented JSON contract: raw counts
// plus the two rates the dashboard renders. Rates are fractions in
// [0,1]; the UI formats them as percentages.
type linkedInMetricsResponse struct {
	InvitesSent    int64   `json:"invites_sent"`
	Accepted       int64   `json:"accepted"`
	AcceptanceRate float64 `json:"acceptance_rate"`
	DMsSent        int64   `json:"dms_sent"`
	Replies        int64   `json:"replies"`
	ReplyRate      float64 `json:"reply_rate"`
}

func callLinkedInMetrics(t *testing.T, h *CampaignHandler, tenantID, campaignID uuid.UUID) (*httptest.ResponseRecorder, linkedInMetricsResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/linkedin-metrics", nil)
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": campaignID.String()})
	rr := httptest.NewRecorder()
	h.LinkedInMetrics(rr, req)
	var resp linkedInMetricsResponse
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode body=%q err=%v", rr.Body.String(), err)
		}
	}
	return rr, resp
}

// Tracer: a LinkedIn campaign that exists but has no events must return
// zero counts and zero rates — and crucially must not divide by zero on
// the empty denominators. Proves route → tenant check → aggregation
// query → rate computation → JSON shape end-to-end in the simplest
// possible state.
func TestLinkedInMetrics_TracerEmptyCampaign(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.InvitesSent != 0 || resp.Accepted != 0 || resp.DMsSent != 0 || resp.Replies != 0 {
		t.Errorf("empty campaign returned non-zero counts: %+v", resp)
	}
	if resp.AcceptanceRate != 0 || resp.ReplyRate != 0 {
		t.Errorf("empty campaign returned non-zero rates: %+v", resp)
	}
}

// Real-flow tracer: a single invite_sent written through the production
// CreateLinkedInEvent call must surface as invites_sent=1, with every
// other count and rate zero. Proves the whole vertical on real bytes —
// real write path → CountCampaignLinkedInEvents FILTER → handler →
// documented JSON contract — in the smallest non-empty state.
func TestLinkedInMetrics_TracerRealEventFlow(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)

	lead := f.addLead()
	f.addEvent(lead, "invite_sent", 0)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.InvitesSent != 1 {
		t.Errorf("invites_sent=%d want 1", resp.InvitesSent)
	}
	if resp.Accepted != 0 || resp.DMsSent != 0 || resp.Replies != 0 {
		t.Errorf("non-invite counts should be zero: %+v", resp)
	}
	if resp.AcceptanceRate != 0 || resp.ReplyRate != 0 {
		t.Errorf("rates should be zero with no accept/reply: %+v", resp)
	}
}

// Counts + rates against a seeded funnel (issue #9 criterion 1). Ten
// invites go out; four are accepted and get a first DM; one of those
// replies. The endpoint must report the raw counts and the two derived
// rates: acceptance 4/10 = 0.4, reply 1/4 = 0.25. Interleaved
// failed/skipped/withdrawn events prove the per-event-type FILTER is
// precise and does not leak non-funnel events into any count.
func TestLinkedInMetrics_CountsAndRates(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)

	// 10 leads, each invited at step 0.
	leads := make([]uuid.UUID, 10)
	for i := range leads {
		leads[i] = f.addLead()
		f.addEvent(leads[i], "invite_sent", 0)
	}
	// 4 accepted → first DM at step 1.
	for i := 0; i < 4; i++ {
		f.addEvent(leads[i], "accepted", 0)
		f.addEvent(leads[i], "dm_sent", 1)
	}
	// 1 of those replied.
	f.addEvent(leads[0], "replied", 1)
	// Noise: these must not move any of the four funnel counts.
	f.addEvent(leads[9], "failed", 0)
	f.addEvent(leads[8], "skipped", 1)
	f.addEvent(leads[7], "withdrawn", 0)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr, resp := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if resp.InvitesSent != 10 || resp.Accepted != 4 || resp.DMsSent != 4 || resp.Replies != 1 {
		t.Errorf("counts wrong: %+v want invites=10 accepted=4 dms=4 replies=1", resp)
	}
	if !almostEqual(resp.AcceptanceRate, 0.4) {
		t.Errorf("acceptance_rate=%v want 0.4 (4/10)", resp.AcceptanceRate)
	}
	if !almostEqual(resp.ReplyRate, 0.25) {
		t.Errorf("reply_rate=%v want 0.25 (1/4)", resp.ReplyRate)
	}
}

// End-to-end loop closure (issue #9 criterion 2): drive the real #7
// accept and #8 reply writes for one lead, then assert the dashboard
// endpoint reflects them. The invite/accept/dm halves go through the same
// CreateLinkedInEvent call the send worker and the reconcile's
// applyLinkedInAccept make; the reply half drives the actual #8 code path
// — suppression.RecordReplyByPerson — which flips the lead to 'replied',
// writes a real 'replied' event, and suppresses the person. Baseline is
// asserted zero first so the after-counts are unambiguous deltas.
func TestLinkedInMetrics_AcceptAndReplyReflected(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)
	h := NewCampaignHandler(repository.New(pool), nil)

	lead := f.addLead()

	_, base := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	if base.InvitesSent != 0 || base.Accepted != 0 || base.DMsSent != 0 || base.Replies != 0 {
		t.Fatalf("baseline not zero: %+v", base)
	}

	// #7: invite went out, the ~45-min connections check matched (accepted),
	// and the first DM that accept schedules was sent.
	f.addEvent(lead, "invite_sent", 0)
	f.addEvent(lead, "accepted", 1)
	f.addEvent(lead, "dm_sent", 1)

	// #8: the prospect replies to that DM — the real reply path.
	supp := suppression.New(pool)
	if err := supp.RecordReplyByPerson(context.Background(), lead, "unipile_msg_"+uuid.NewString()); err != nil {
		t.Fatalf("RecordReplyByPerson: %v", err)
	}

	_, after := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	if after.InvitesSent != 1 || after.Accepted != 1 || after.DMsSent != 1 || after.Replies != 1 {
		t.Errorf("dashboard did not reflect accept+reply: %+v want invites=1 accepted=1 dms=1 replies=1", after)
	}
	if !almostEqual(after.AcceptanceRate, 1.0) {
		t.Errorf("acceptance_rate=%v want 1.0 (1 accepted / 1 invite)", after.AcceptanceRate)
	}
	if !almostEqual(after.ReplyRate, 1.0) {
		t.Errorf("reply_rate=%v want 1.0 (1 reply / 1 dm)", after.ReplyRate)
	}
}

// Same-source consistency (issue #9 criterion 4): the dashboard's
// acceptance_rate and the pacer breaker's acceptance rate must read the
// same underlying signal — the same linkedin_events table, the same
// 'invite_sent'/'accepted' literals, the same accepted÷invites formula.
// The two queries deliberately differ in SCOPE (dashboard = per-campaign,
// all-time via CountCampaignLinkedInEvents; breaker = per-account,
// trailing-7-day via GetLinkedInAcceptanceStats), so we align the scopes
// here — one account bound 1:1 to one campaign, all events fresh — and
// prove the counts and the ratio coincide. If either side ever drifts to a
// different event literal or a different numerator/denominator, this fails.
func TestLinkedInMetrics_AcceptanceRateMatchesBreakerSource(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)
	q := repository.New(pool)

	account := f.addAccount()
	// 10 invites from this account; 4 accepted. Every lead is bound to the
	// account and under the fixture campaign, so account-scope and
	// campaign-scope cover the same events.
	leads := make([]uuid.UUID, 10)
	for i := range leads {
		leads[i] = f.addLeadForAccount(account)
		f.addEvent(leads[i], "invite_sent", 0)
	}
	for i := 0; i < 4; i++ {
		f.addEvent(leads[i], "accepted", 1)
	}

	ctx := context.Background()
	// Breaker source (pacer, issue #7): per-account, trailing 7 days.
	stats, err := q.GetLinkedInAcceptanceStats(ctx, pgUUID(account))
	if err != nil {
		t.Fatalf("GetLinkedInAcceptanceStats: %v", err)
	}
	// Dashboard source: per-campaign counts behind LinkedInMetrics.
	counts, err := q.CountCampaignLinkedInEvents(ctx, pgUUID(f.campaignID))
	if err != nil {
		t.Fatalf("CountCampaignLinkedInEvents: %v", err)
	}
	if stats.InvitesSent != counts.InvitesSent || stats.Accepted != counts.Accepted {
		t.Errorf("breaker vs dashboard source disagree: breaker{inv=%d acc=%d} dashboard{inv=%d acc=%d}",
			stats.InvitesSent, stats.Accepted, counts.InvitesSent, counts.Accepted)
	}
	if stats.InvitesSent != 10 || stats.Accepted != 4 {
		t.Fatalf("unexpected counts: inv=%d acc=%d want 10/4", stats.InvitesSent, stats.Accepted)
	}

	// The number the dashboard renders equals the number the breaker divides.
	h := NewCampaignHandler(q, nil)
	_, resp := callLinkedInMetrics(t, h, f.tenantID, f.campaignID)
	breakerRate := float64(stats.Accepted) / float64(stats.InvitesSent)
	if !almostEqual(resp.AcceptanceRate, breakerRate) || !almostEqual(resp.AcceptanceRate, 0.4) {
		t.Errorf("dashboard acceptance_rate=%v want %v (== breaker rate == 0.4)", resp.AcceptanceRate, breakerRate)
	}
}

// Security: a tenant that does not own this campaign gets 404 from the
// LinkedIn metrics endpoint too — same invariant as the email metrics
// route. Cross-tenant campaign IDs must never leak.
func TestLinkedInMetrics_TenantScopeRejects(t *testing.T) {
	pool := withMetricsPool(t)
	f := newLinkedInMetricsFixture(t, pool)

	h := NewCampaignHandler(repository.New(pool), nil)
	other := uuid.New()
	rr, _ := callLinkedInMetrics(t, h, other, f.campaignID)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", rr.Code, rr.Body.String())
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
