//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/gmail/poller"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// pollableFixture builds the full graph a poll-tick needs: a tenant
// with a connected gmail_account (carrying a stale cursor), a
// campaign with one campaign_lead whose `sent` email_event has a
// gmail_message_id the SentLookup callback resolves back to it.
type pollableFixture struct {
	tenantID       uuid.UUID
	gmailAccountID uuid.UUID
	leadID         uuid.UUID
	sentMsgID      string
	email          string
}

func newPollableFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) pollableFixture {
	t.Helper()
	f := pollableFixture{
		tenantID:       uuid.New(),
		gmailAccountID: uuid.New(),
		leadID:         uuid.New(),
		sentMsgID:      "<sent-" + uuid.NewString() + "@mail.gmail.com>",
		email:          "lead-" + uuid.NewString() + "@example.com",
	}
	userID := uuid.New()
	personID := uuid.New()
	emailRowID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	eventID := uuid.New()
	clerkID := "user_test_" + uuid.NewString()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	mustExec(`INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		userID, clerkID, "owner-"+uuid.NewString()+"@example.com")
	mustExec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "Poll Test "+uuid.NewString())
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Poll Lead", "poll lead", "Poll", "Lead")
	mustExec(`INSERT INTO emails (id, email, person_id) VALUES ($1, $2, $3)`,
		emailRowID, f.email, personID)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['email']::text[])`,
		playID, f.tenantID, "Poll Test Play")
	seq, _ := json.Marshal([]SequenceStep{{Step: 1, DelayDays: 1, Subject: "s", Body: "b"}})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, sequence) VALUES ($1, $2, $3, $4, 'active', $5::jsonb)`,
		campaignID, f.tenantID, playID, "Poll Test Campaign", seq)
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step) VALUES ($1, $2, $3, 'active', 1)`,
		f.leadID, campaignID, personID)
	// The 'sent' event whose gmail_message_id is what the poller's
	// SentLookup must resolve back to this campaign_lead.
	mustExec(`INSERT INTO email_events (id, campaign_lead_id, event_type, step, gmail_message_id, metadata) VALUES ($1, $2, 'sent', 1, $3, '{}'::jsonb)`,
		eventID, f.leadID, f.sentMsgID)
	// Connected Gmail account with a non-empty cursor so the tick
	// uses history.list (not bootstrap).
	mustExec(`INSERT INTO gmail_accounts (id, tenant_id, user_id, email, access_token, refresh_token, token_expiry, last_history_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.gmailAccountID, f.tenantID, userID, "operator@example.com", "at", "rt", time.Now().Add(time.Hour), "100")

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM email_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM emails WHERE id = $1`, emailRowID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM gmail_accounts WHERE id = $1`, f.gmailAccountID)
		_, _ = pool.Exec(c, `DELETE FROM unsubscribes WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// TestPollOnce_ReplyRecorded is the worker-tick tracer: a stubbed
// poller returns one ClassifiedEvent{Kind: reply} → pollOnce calls
// suppression.RecordReply → campaign_lead.status flips to 'replied',
// a 'replied' email_event lands with the inbound gmail_message_id,
// and an unsubscribes row with reason='reply' is inserted (so the
// "zero emails to replied leads" invariant is enforced by the same
// IsSuppressed gate the sender already checks).
func TestPollOnce_ReplyRecorded(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	const inboundMsgID = "<reply-inbound-1@mail.example.com>"

	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return "205", []poller.ClassifiedEvent{
			{
				Kind:             poller.KindReply,
				InboundMessageID: inboundMsgID,
				InReplyTo:        f.sentMsgID,
				CampaignLeadID:   f.leadID,
			},
		}, nil
	}

	pollOnce(ctx, repository.New(pool), suppression.New(pool), poll)

	// Lead flipped to replied.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead status: %v", err)
	}
	if status != "replied" {
		t.Errorf("campaign_leads.status=%q want 'replied'", status)
	}

	// 'replied' email_event written with the INBOUND msg-id (not the
	// sent one — the inbound is the new evidence; the sent one is
	// already on the 'sent' event row).
	var repliedCount int
	var gotMsgID *string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(gmail_message_id)
		FROM email_events
		WHERE campaign_lead_id = $1 AND event_type = 'replied'
	`, f.leadID).Scan(&repliedCount, &gotMsgID); err != nil {
		t.Fatalf("scan replied event: %v", err)
	}
	if repliedCount != 1 {
		t.Errorf("replied events=%d want 1", repliedCount)
	}
	if gotMsgID == nil || *gotMsgID != inboundMsgID {
		got := "<nil>"
		if gotMsgID != nil {
			got = *gotMsgID
		}
		t.Errorf("replied event gmail_message_id=%q want %q", got, inboundMsgID)
	}

	// Suppression row exists with reason='reply' — the IsSuppressed
	// gate the sender already checks will now block any future send
	// to this address.
	var unsubReason string
	if err := pool.QueryRow(ctx, `
		SELECT reason FROM unsubscribes
		WHERE tenant_id = $1 AND lower(email) = lower($2)
	`, f.tenantID, f.email).Scan(&unsubReason); err != nil {
		t.Fatalf("scan unsubscribes: %v", err)
	}
	if unsubReason != suppression.ReasonReply {
		t.Errorf("unsubscribes.reason=%q want %q", unsubReason, suppression.ReasonReply)
	}
}

// TestPollOnce_CursorPersisted — on a successful tick (even with no
// events), the new cursor and last_polled_at are written so the next
// tick resumes from there. Without persistence, every tick replays
// from the same point.
func TestPollOnce_CursorPersisted(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	const newCursor = "987654"

	beforePoll := time.Now().Add(-time.Second)
	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return newCursor, nil, nil
	}
	pollOnce(ctx, repository.New(pool), suppression.New(pool), poll)

	var gotCursor *string
	var gotPolled *time.Time
	if err := pool.QueryRow(ctx, `SELECT last_history_id, last_polled_at FROM gmail_accounts WHERE id = $1`, f.gmailAccountID).Scan(&gotCursor, &gotPolled); err != nil {
		t.Fatalf("scan account: %v", err)
	}
	if gotCursor == nil || *gotCursor != newCursor {
		got := "<nil>"
		if gotCursor != nil {
			got = *gotCursor
		}
		t.Errorf("last_history_id=%q want %q", got, newCursor)
	}
	if gotPolled == nil || gotPolled.Before(beforePoll) {
		t.Errorf("last_polled_at=%v not advanced past %v", gotPolled, beforePoll)
	}
}

// TestPollOnce_CursorNotAdvancedOnError — a poller error (transient
// outage) must NOT advance the cursor; otherwise the next tick would
// skip the unprocessed window and lose any reply that lands inside
// it.
func TestPollOnce_CursorNotAdvancedOnError(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	originalCursor := "100" // matches the fixture default

	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return "", nil, fmt.Errorf("%w: simulated outage", poller.ErrRateLimited)
	}
	pollOnce(ctx, repository.New(pool), suppression.New(pool), poll)

	var gotCursor *string
	if err := pool.QueryRow(ctx, `SELECT last_history_id FROM gmail_accounts WHERE id = $1`, f.gmailAccountID).Scan(&gotCursor); err != nil {
		t.Fatalf("scan cursor: %v", err)
	}
	if gotCursor == nil || *gotCursor != originalCursor {
		got := "<nil>"
		if gotCursor != nil {
			got = *gotCursor
		}
		t.Errorf("cursor advanced despite error: got=%q want=%q", got, originalCursor)
	}
}

// TestPollOnce_TokenExpiredLogged — when the poller returns
// ErrTokenExpired (refresh token revoked), the tick must not crash,
// must not advance the cursor, and must not call into suppression.
// The UI's "needs reconnect" surface is in #8; this tick just logs.
func TestPollOnce_TokenExpiredLogged(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	originalCursor := "100"

	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return "", nil, fmt.Errorf("%w: revoked", poller.ErrTokenExpired)
	}
	// pollOnce should not panic / not advance state.
	pollOnce(ctx, repository.New(pool), suppression.New(pool), poll)

	var gotCursor *string
	if err := pool.QueryRow(ctx, `SELECT last_history_id FROM gmail_accounts WHERE id = $1`, f.gmailAccountID).Scan(&gotCursor); err != nil {
		t.Fatalf("scan cursor: %v", err)
	}
	if gotCursor == nil || *gotCursor != originalCursor {
		got := "<nil>"
		if gotCursor != nil {
			got = *gotCursor
		}
		t.Errorf("cursor advanced despite ErrTokenExpired: got=%q want=%q", got, originalCursor)
	}
	// No 'replied' events fired.
	var repliedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM email_events WHERE campaign_lead_id = $1 AND event_type='replied'`, f.leadID).Scan(&repliedCount); err != nil {
		t.Fatalf("scan replied count: %v", err)
	}
	if repliedCount != 0 {
		t.Errorf("unexpected replied events on error path: %d", repliedCount)
	}
}

// TestPollOnce_HardBounceRecorded is the issue-#5 tracer at the
// worker layer: a stubbed poller emits one KindBounceHard with the
// DSN status code and recipient resolved to a known campaign_lead.
// pollOnce must, in one tick:
//
//   - write a 'bounced' email_event row whose JSONB metadata carries
//     the DSN status code captured by the classifier,
//   - flip the campaign_lead status to 'bounced' so the sender's
//     GetDueLeads (which filters status='active') stops returning it,
//   - insert a tenant-scoped unsubscribes row with reason='hard-bounce',
//     and
//   - the next sender-side IsSuppressed gate must report blocked for
//     the bounced address so even an out-of-band campaign on the same
//     tenant skips it.
func TestPollOnce_HardBounceRecorded(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)

	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return "300", []poller.ClassifiedEvent{
			{
				Kind:              poller.KindBounceHard,
				InboundMessageID:  "<bounce-hard-1@x>",
				InReplyTo:         f.sentMsgID,
				CampaignLeadID:    f.leadID,
				DSNStatusCode:     "5.1.1",
				OriginalRecipient: f.email,
			},
		}, nil
	}
	supp := suppression.New(pool)
	pollOnce(ctx, repository.New(pool), supp, poll)

	// campaign_lead halted via status='bounced'.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "bounced" {
		t.Errorf("campaign_leads.status=%q want 'bounced'", status)
	}

	// 'bounced' email_event written with the DSN status code in
	// metadata — the audit trail must capture *why* the bounce
	// happened, not just *that* it happened.
	var meta []byte
	if err := pool.QueryRow(ctx, `
		SELECT metadata FROM email_events
		WHERE campaign_lead_id=$1 AND event_type='bounced'
		ORDER BY created_at DESC LIMIT 1
	`, f.leadID).Scan(&meta); err != nil {
		t.Fatalf("scan bounced event: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(meta, &parsed); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if parsed["dsn_status_code"] != "5.1.1" {
		t.Errorf("metadata.dsn_status_code=%v want \"5.1.1\"", parsed["dsn_status_code"])
	}

	// unsubscribes row with reason='hard-bounce'.
	var unsubReason string
	if err := pool.QueryRow(ctx, `
		SELECT reason FROM unsubscribes
		WHERE tenant_id=$1 AND lower(email)=lower($2)
	`, f.tenantID, f.email).Scan(&unsubReason); err != nil {
		t.Fatalf("scan unsubscribes: %v", err)
	}
	if unsubReason != suppression.ReasonHardBounce {
		t.Errorf("unsubscribes.reason=%q want %q", unsubReason, suppression.ReasonHardBounce)
	}

	// Sender-side IsSuppressed reports blocked — the contract is
	// upheld for every subsequent send across the tenant, including
	// new campaigns that haven't seen this address before.
	suppressed, reason, err := supp.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !suppressed || reason != suppression.ReasonHardBounce {
		t.Errorf("IsSuppressed=(%t,%q) want (true, %q)", suppressed, reason, suppression.ReasonHardBounce)
	}
}

// TestPollOnce_SoftBounceBelowThreshold — a single soft bounce
// writes the 'soft-bounce' email_event (with DSN status code +
// count=1 in metadata) but does NOT halt the lead and does NOT
// insert an unsubscribes row. The threshold is three consecutive;
// one bounce is not lead-terminal.
func TestPollOnce_SoftBounceBelowThreshold(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)

	poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
		return "310", []poller.ClassifiedEvent{
			{
				Kind:              poller.KindBounceSoft,
				InboundMessageID:  "<bounce-soft-1@x>",
				InReplyTo:         f.sentMsgID,
				CampaignLeadID:    f.leadID,
				DSNStatusCode:     "4.2.2",
				OriginalRecipient: f.email,
			},
		}, nil
	}
	supp := suppression.New(pool)
	pollOnce(ctx, repository.New(pool), supp, poll)

	// Lead status untouched — soft below threshold is not terminal.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "active" {
		t.Errorf("campaign_leads.status=%q want 'active' (soft below threshold should not halt)", status)
	}

	// 'soft-bounce' event written; metadata carries status code + count.
	var meta []byte
	if err := pool.QueryRow(ctx, `
		SELECT metadata FROM email_events
		WHERE campaign_lead_id=$1 AND event_type='soft-bounce'
		ORDER BY created_at DESC LIMIT 1
	`, f.leadID).Scan(&meta); err != nil {
		t.Fatalf("scan soft-bounce event: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(meta, &parsed); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if parsed["dsn_status_code"] != "4.2.2" {
		t.Errorf("metadata.dsn_status_code=%v want \"4.2.2\"", parsed["dsn_status_code"])
	}
	if got := parsed["count"]; got == nil {
		t.Errorf("metadata missing count")
	} else {
		// JSON-decoded number arrives as float64.
		if f64, ok := got.(float64); !ok || int64(f64) != 1 {
			t.Errorf("metadata.count=%v want 1", got)
		}
	}

	// No unsubscribes row yet — still below threshold.
	var unsubCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM unsubscribes WHERE tenant_id=$1 AND lower(email)=lower($2)`,
		f.tenantID, f.email).Scan(&unsubCount); err != nil {
		t.Fatalf("scan unsubscribes: %v", err)
	}
	if unsubCount != 0 {
		t.Errorf("unsubscribes rows=%d want 0 below threshold", unsubCount)
	}
}

// TestPollOnce_SoftBounceReachesThreshold — three consecutive soft
// bounces (delivered across three poller ticks, no intervening
// 'sent') trip the suppression-module threshold. The third tick
// writes the unsubscribes row; the next IsSuppressed check reports
// the address blocked with reason='soft-bounce-threshold'. This is
// the invariant the issue calls out: "subsequent worker ticks skip
// that lead via IsSuppressed."
func TestPollOnce_SoftBounceReachesThreshold(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	supp := suppression.New(pool)

	for i := 0; i < suppression.SoftBounceThreshold; i++ {
		i := i
		poll := func(_ context.Context, _ poller.Account) (string, []poller.ClassifiedEvent, error) {
			return fmt.Sprintf("4%02d", i), []poller.ClassifiedEvent{
				{
					Kind:              poller.KindBounceSoft,
					InboundMessageID:  fmt.Sprintf("<bounce-soft-%d@x>", i),
					InReplyTo:         f.sentMsgID,
					CampaignLeadID:    f.leadID,
					DSNStatusCode:     "4.2.2",
					OriginalRecipient: f.email,
				},
			}, nil
		}
		pollOnce(ctx, repository.New(pool), supp, poll)
	}

	suppressed, reason, err := supp.IsSuppressed(ctx, f.tenantID, f.email)
	if err != nil {
		t.Fatalf("IsSuppressed: %v", err)
	}
	if !suppressed || reason != suppression.ReasonSoftBounceThreshold {
		t.Errorf("after %d ticks IsSuppressed=(%t,%q) want (true,%q)",
			suppression.SoftBounceThreshold, suppressed, reason, suppression.ReasonSoftBounceThreshold)
	}
}

// sentLookupMatches is a guard that pins the SentLookup factory's
// contract: when the DB has an email_events row with the queried
// gmail_message_id, return (lead, true, nil); when it doesn't,
// return (Nil, false, nil) — no error — so the poller's classifier
// treats unknown ids as "not us" rather than failing the whole tick.
func TestNewSentLookup_Contract(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	f := newPollableFixture(t, ctx, pool)
	lookup := NewSentLookup(repository.New(pool))

	leadID, ok, err := lookup(ctx, f.sentMsgID)
	if err != nil {
		t.Fatalf("lookup known msg-id: %v", err)
	}
	if !ok {
		t.Errorf("ok=false for known msg-id %q", f.sentMsgID)
	}
	if leadID != f.leadID {
		t.Errorf("leadID=%v want %v", leadID, f.leadID)
	}

	leadID, ok, err = lookup(ctx, "<unknown@nowhere>")
	if err != nil {
		t.Fatalf("lookup unknown msg-id: %v", err)
	}
	if ok {
		t.Errorf("ok=true for unknown msg-id; leadID=%v", leadID)
	}
}

// quiet the unused-import linter when assertions don't reference
// errors. Kept as a guard: future test edits that drop errors.Is
// usage won't break the build.
var _ = errors.Is
