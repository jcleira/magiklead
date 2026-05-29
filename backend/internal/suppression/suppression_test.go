//go:build integration

package suppression_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/suppression/...

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

// fixture is the graph each test needs: tenant + user + play + campaign +
// campaign_lead + (person + email) so the resolver-style queries return
// hits. Built fresh per-test, torn down via t.Cleanup.
type fixture struct {
	t              *testing.T
	pool           *pgxpool.Pool
	tenantID       uuid.UUID
	userID         uuid.UUID
	personID       uuid.UUID
	emailID        uuid.UUID
	playID         uuid.UUID
	campaignID     uuid.UUID
	campaignLeadID uuid.UUID
	email          string
}

func newFixture(t *testing.T, pool *pgxpool.Pool, email string) *fixture {
	t.Helper()
	if email == "" {
		email = "lead-" + uuid.NewString() + "@example.com"
	}
	f := &fixture{
		t:              t,
		pool:           pool,
		tenantID:       uuid.New(),
		userID:         uuid.New(),
		personID:       uuid.New(),
		emailID:        uuid.New(),
		playID:         uuid.New(),
		campaignID:     uuid.New(),
		campaignLeadID: uuid.New(),
		email:          email,
	}

	ctx := context.Background()
	clerkID := "user_test_" + uuid.NewString()
	mustExec(t, pool, `INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		f.userID, clerkID, "owner-"+uuid.NewString()+"@example.com")
	mustExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "Suppression Test "+uuid.NewString())
	mustExec(t, pool, `INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		f.userID, f.tenantID)
	mustExec(t, pool, `INSERT INTO persons (id, canonical_name, normalized_name) VALUES ($1, $2, $3)`,
		f.personID, "Test Lead", "test lead")
	mustExec(t, pool, `INSERT INTO emails (id, email, person_id) VALUES ($1, $2, $3)`,
		f.emailID, email, f.personID)
	mustExec(t, pool, `INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['email']::text[])`,
		f.playID, f.tenantID, "Test Play")
	mustExec(t, pool, `INSERT INTO campaigns (id, tenant_id, play_id, name, sequence) VALUES ($1, $2, $3, $4, '[]'::jsonb)`,
		f.campaignID, f.tenantID, f.playID, "Test Campaign")
	mustExec(t, pool, `INSERT INTO campaign_leads (id, campaign_id, person_id, status) VALUES ($1, $2, $3, 'active')`,
		f.campaignLeadID, f.campaignID, f.personID)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM email_events WHERE campaign_lead_id = $1`, f.campaignLeadID)
		_, _ = pool.Exec(ctx, `DELETE FROM campaign_leads WHERE id = $1`, f.campaignLeadID)
		_, _ = pool.Exec(ctx, `DELETE FROM campaigns WHERE id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM plays WHERE id = $1`, f.playID)
		_, _ = pool.Exec(ctx, `DELETE FROM emails WHERE id = $1`, f.emailID)
		_, _ = pool.Exec(ctx, `DELETE FROM persons WHERE id = $1`, f.personID)
		_, _ = pool.Exec(ctx, `DELETE FROM unsubscribes WHERE tenant_id = $1 OR email = $2`, f.tenantID, email)
		_, _ = pool.Exec(ctx, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.userID)
	})

	return f
}

func mustExec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// Tracer bullet: with an empty graph, IsSuppressed returns false. This
// proves the migration → query → module path is wired before any of
// the branch tests touch state.
func TestIsSuppressed_TracerEmptyState(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if ok || reason != "" {
		t.Errorf("expected (false, \"\"), got (%t, %q)", ok, reason)
	}
}

func TestIsSuppressed_EmptyEmail(t *testing.T) {
	pool := withPool(t)
	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), uuid.New(), "")
	if err != nil {
		t.Fatalf("IsSuppressed empty: %v", err)
	}
	if ok || reason != "" {
		t.Errorf("empty email should not suppress, got (%t, %q)", ok, reason)
	}
}

func TestIsSuppressed_TenantUnsubscribe(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	mustExec(t, pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES ($1, $2, $3)`,
		f.tenantID, f.email, suppression.ReasonManual)

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonManual {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonManual, ok, reason)
	}

	// Same email but a different tenant must NOT be suppressed —
	// tenant-scoped rows must not leak across tenants.
	otherTenant := uuid.New()
	ok, _, err = m.IsSuppressed(context.Background(), otherTenant, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed other tenant: %v", err)
	}
	if ok {
		t.Error("tenant-scoped suppression leaked across tenants")
	}
}

func TestIsSuppressed_GlobalUnsubscribe(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	mustExec(t, pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES (NULL, $1, $2)`,
		f.email, suppression.ReasonSpamComplaint)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM unsubscribes WHERE email = $1 AND tenant_id IS NULL`, f.email)
	})

	m := suppression.New(pool)
	// Any tenant (even one unrelated to the fixture) must see global
	// suppression hit.
	for _, tenant := range []uuid.UUID{f.tenantID, uuid.New()} {
		ok, reason, err := m.IsSuppressed(context.Background(), tenant, f.email)
		if err != nil {
			t.Fatalf("IsSuppressed tenant=%s: %v", tenant, err)
		}
		if !ok || reason != suppression.ReasonSpamComplaint {
			t.Errorf("tenant=%s expected (true, %q), got (%t, %q)",
				tenant, suppression.ReasonSpamComplaint, ok, reason)
		}
	}
}

func TestIsSuppressed_PrefersTenantOverGlobal(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	mustExec(t, pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES (NULL, $1, $2)`,
		f.email, suppression.ReasonSpamComplaint)
	mustExec(t, pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES ($1, $2, $3)`,
		f.tenantID, f.email, suppression.ReasonManual)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM unsubscribes WHERE email = $1`, f.email)
	})

	m := suppression.New(pool)
	_, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if reason != suppression.ReasonManual {
		t.Errorf("reason=%q want %q (tenant row should win over global)", reason, suppression.ReasonManual)
	}
}

func TestIsSuppressed_CaseInsensitiveEmail(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	mustExec(t, pool, `INSERT INTO unsubscribes (tenant_id, email, reason) VALUES ($1, $2, $3)`,
		f.tenantID, "Mixed.CASE."+f.email, suppression.ReasonManual)

	// Query with all-lowercase variant — the lower(email) index must
	// catch it.
	m := suppression.New(pool)
	ok, _, err := m.IsSuppressed(context.Background(), f.tenantID, "mixed.case."+f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok {
		t.Error("case-insensitive match failed")
	}
}

func TestIsSuppressed_HardBounceEvent(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	// Write the email_event directly — no unsubscribes row. The check
	// must hit layer 2 of IsSuppressed (HasHardBounceEvent).
	mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step) VALUES ($1, 'bounced', 1)`,
		f.campaignLeadID)

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonHardBounce {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonHardBounce, ok, reason)
	}
}

func TestIsSuppressed_SoftBounceBelowThreshold(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	for i := 0; i < suppression.SoftBounceThreshold-1; i++ {
		mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step) VALUES ($1, 'soft-bounce', 1)`,
			f.campaignLeadID)
	}

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if ok {
		t.Errorf("count below threshold should not suppress (got reason %q)", reason)
	}
}

func TestIsSuppressed_SoftBounceAtThreshold(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	for i := 0; i < suppression.SoftBounceThreshold; i++ {
		mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step) VALUES ($1, 'soft-bounce', 1)`,
			f.campaignLeadID)
	}

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(context.Background(), f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonSoftBounceThreshold {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonSoftBounceThreshold, ok, reason)
	}
}

func TestIsSuppressed_SoftBounceResetsAfterSent(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()
	// Two soft bounces, then a successful send, then one more soft —
	// the streak resets after 'sent', so we're at 1, below threshold.
	mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step, created_at) VALUES ($1, 'soft-bounce', 1, NOW() - INTERVAL '3 hours')`, f.campaignLeadID)
	mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step, created_at) VALUES ($1, 'soft-bounce', 1, NOW() - INTERVAL '2 hours')`, f.campaignLeadID)
	mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step, created_at) VALUES ($1, 'sent', 1, NOW() - INTERVAL '1 hour')`, f.campaignLeadID)
	mustExec(t, pool, `INSERT INTO email_events (campaign_lead_id, event_type, step, created_at) VALUES ($1, 'soft-bounce', 2, NOW())`, f.campaignLeadID)

	m := suppression.New(pool)
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if ok {
		t.Errorf("send should have reset streak (got reason %q)", reason)
	}
}

func TestRecordReply(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	if err := m.RecordReply(ctx, f.campaignLeadID, "<gmail-msg-123@mail.google.com>"); err != nil {
		t.Fatalf("RecordReply: %v", err)
	}

	// email_events row written with type='replied' and the message-id.
	q := repository.New(pool)
	events, err := q.ListEmailEvents(ctx, pgUUID(f.campaignLeadID))
	if err != nil {
		t.Fatalf("ListEmailEvents: %v", err)
	}
	var replied *repository.EmailEvent
	for i := range events {
		if events[i].EventType == "replied" {
			replied = &events[i]
			break
		}
	}
	if replied == nil {
		t.Fatal("no 'replied' email_events row written")
	}
	if !replied.GmailMessageID.Valid || replied.GmailMessageID.String != "<gmail-msg-123@mail.google.com>" {
		t.Errorf("gmail_message_id=%+v want '<gmail-msg-123@mail.google.com>'", replied.GmailMessageID)
	}
	var metaCheck map[string]string
	if err := json.Unmarshal(replied.Metadata, &metaCheck); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if metaCheck["gmail_message_id"] != "<gmail-msg-123@mail.google.com>" {
		t.Errorf("metadata gmail_message_id=%q", metaCheck["gmail_message_id"])
	}

	// campaign_lead status flipped to 'replied'.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.campaignLeadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "replied" {
		t.Errorf("campaign_leads.status=%q want 'replied'", status)
	}

	// unsubscribes row created so future sequences from this tenant
	// don't re-target.
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonReply {
		t.Errorf("after RecordReply expected (true, %q), got (%t, %q)",
			suppression.ReasonReply, ok, reason)
	}
}

// TestClearReply is the re-engage round-trip: RecordReply puts the
// lead in the exact state the poller produces (suppressed +
// status='replied'), then ClearReply must lift the reply gate and
// flip the lead back to 'active' so the worker resumes the sequence.
func TestClearReply(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()
	m := suppression.New(pool)

	// Arrive at the replied state the way production does.
	if err := m.RecordReply(ctx, f.campaignLeadID, "<gmail-inbound-1@mail.google.com>"); err != nil {
		t.Fatalf("RecordReply: %v", err)
	}
	if ok, _, err := m.IsSuppressed(ctx, f.tenantID, f.email); err != nil || !ok {
		t.Fatalf("precondition: expected suppressed after RecordReply (ok=%t err=%v)", ok, err)
	}

	if err := m.ClearReply(ctx, f.campaignLeadID); err != nil {
		t.Fatalf("ClearReply: %v", err)
	}

	// Reply gate lifted — the sender's IsSuppressed check now passes.
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed after ClearReply: %v", err)
	}
	if ok {
		t.Errorf("still suppressed after ClearReply (reason=%q)", reason)
	}

	// Lead reactivated so the next worker tick picks it up.
	var status string
	var nextSendAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, next_send_at FROM campaign_leads WHERE id = $1`, f.campaignLeadID).Scan(&status, &nextSendAt); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" {
		t.Errorf("campaign_leads.status=%q want 'active'", status)
	}
	if nextSendAt == nil {
		t.Errorf("next_send_at is NULL — worker won't pick the lead up")
	}
}

// TestClearReply_DoesNotLiftHardBounce pins the safety invariant: a
// re-engage only clears the *reply* gate. A hard-bounce suppression
// on the same address must survive — re-sending to a dead mailbox
// would burn the tenant's sender reputation.
func TestClearReply_DoesNotLiftHardBounce(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()
	m := suppression.New(pool)

	if err := m.RecordReply(ctx, f.campaignLeadID, "<inbound@x>"); err != nil {
		t.Fatalf("RecordReply: %v", err)
	}
	// Same address also hard-bounced (e.g. a later step before the
	// reply was processed) — independent suppression vector.
	if err := m.RecordHardBounce(ctx, f.email, f.tenantID, nil); err != nil {
		t.Fatalf("RecordHardBounce: %v", err)
	}

	if err := m.ClearReply(ctx, f.campaignLeadID); err != nil {
		t.Fatalf("ClearReply: %v", err)
	}

	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok {
		t.Fatalf("hard-bounce suppression wrongly lifted by ClearReply")
	}
	if reason != suppression.ReasonHardBounce {
		t.Errorf("reason=%q want %q (reply row cleared, hard-bounce kept)", reason, suppression.ReasonHardBounce)
	}
}

func TestRecordHardBounce(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	if err := m.RecordHardBounce(ctx, f.email, f.tenantID, nil); err != nil {
		t.Fatalf("RecordHardBounce: %v", err)
	}

	// email_events 'bounced' row written for the resolved campaign_lead.
	q := repository.New(pool)
	events, err := q.ListEmailEvents(ctx, pgUUID(f.campaignLeadID))
	if err != nil {
		t.Fatalf("ListEmailEvents: %v", err)
	}
	if !containsEvent(events, "bounced") {
		t.Errorf("no 'bounced' event written: %+v", eventTypes(events))
	}

	// unsubscribes row with reason='hard-bounce'.
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonHardBounce {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonHardBounce, ok, reason)
	}
}

func TestRecordHardBounce_EmptyEmail(t *testing.T) {
	pool := withPool(t)
	m := suppression.New(pool)
	err := m.RecordHardBounce(context.Background(), "", uuid.New(), nil)
	if err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestRecordSoftBounce_BelowThreshold(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	for i := 0; i < suppression.SoftBounceThreshold-1; i++ {
		count, err := m.RecordSoftBounce(ctx, f.email, f.tenantID, nil)
		if err != nil {
			t.Fatalf("RecordSoftBounce #%d: %v", i, err)
		}
		if want := int64(i + 1); count != want {
			t.Errorf("RecordSoftBounce #%d returned count=%d want %d", i, count, want)
		}
	}

	// No unsubscribes row yet — still under threshold.
	q := repository.New(pool)
	_, err := q.LookupSuppression(ctx, repository.LookupSuppressionParams{
		TenantID: pgUUID(f.tenantID),
		Email:    f.email,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("unsubscribes row written below threshold (err=%v)", err)
	}
}

func TestRecordSoftBounce_ReachesThreshold(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	var lastCount int64
	for i := 0; i < suppression.SoftBounceThreshold; i++ {
		count, err := m.RecordSoftBounce(ctx, f.email, f.tenantID, nil)
		if err != nil {
			t.Fatalf("RecordSoftBounce #%d: %v", i, err)
		}
		lastCount = count
	}
	if lastCount != int64(suppression.SoftBounceThreshold) {
		t.Errorf("final count=%d want %d", lastCount, suppression.SoftBounceThreshold)
	}

	// IsSuppressed must report soft-bounce-threshold; layer 1 catches
	// the unsubscribes row that the recorder wrote at threshold.
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonSoftBounceThreshold {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonSoftBounceThreshold, ok, reason)
	}
}

func TestRecordUnsubscribe_Tenant(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	tenant := f.tenantID
	if err := m.RecordUnsubscribe(ctx, f.email, &tenant, suppression.ReasonListUnsubscribe); err != nil {
		t.Fatalf("RecordUnsubscribe: %v", err)
	}
	ok, reason, err := m.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonListUnsubscribe {
		t.Errorf("expected (true, %q), got (%t, %q)", suppression.ReasonListUnsubscribe, ok, reason)
	}

	// Other tenants are unaffected.
	ok, _, err = m.IsSuppressed(ctx, uuid.New(), f.email)
	if err != nil {
		t.Fatalf("IsSuppressed other: %v", err)
	}
	if ok {
		t.Error("tenant-scoped unsubscribe leaked to other tenant")
	}
}

func TestRecordUnsubscribe_Global(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM unsubscribes WHERE email = $1 AND tenant_id IS NULL`, f.email)
	})

	m := suppression.New(pool)
	if err := m.RecordUnsubscribe(ctx, f.email, nil, suppression.ReasonSpamComplaint); err != nil {
		t.Fatalf("RecordUnsubscribe(nil tenant): %v", err)
	}
	// Any tenant sees this hit.
	ok, reason, err := m.IsSuppressed(ctx, uuid.New(), f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !ok || reason != suppression.ReasonSpamComplaint {
		t.Errorf("global unsub: expected (true, %q), got (%t, %q)",
			suppression.ReasonSpamComplaint, ok, reason)
	}
}

func TestRecordUnsubscribe_Idempotent(t *testing.T) {
	pool := withPool(t)
	f := newFixture(t, pool, "")
	ctx := context.Background()

	m := suppression.New(pool)
	tenant := f.tenantID
	for i := 0; i < 3; i++ {
		if err := m.RecordUnsubscribe(ctx, f.email, &tenant, suppression.ReasonManual); err != nil {
			t.Fatalf("RecordUnsubscribe #%d: %v", i, err)
		}
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM unsubscribes WHERE tenant_id = $1 AND lower(email) = lower($2)`,
		f.tenantID, f.email).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count=%d want 1 — ON CONFLICT did not collapse duplicates", n)
	}
}

func containsEvent(events []repository.EmailEvent, eventType string) bool {
	for _, e := range events {
		if e.EventType == eventType {
			return true
		}
	}
	return false
}

func eventTypes(events []repository.EmailEvent) []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.EventType)
	}
	return out
}
