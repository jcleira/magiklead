//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// testUnsubCfg returns a worker.UnsubscribeConfig wired with a fixed
// secret + dummy URLs — enough to build valid List-Unsubscribe headers
// in tests without coupling to env vars.
func testUnsubCfg() UnsubscribeConfig {
	return UnsubscribeConfig{
		Secret:     []byte("worker-test-unsub-secret"),
		AppURL:     "http://api.test",
		MailDomain: "mail.test",
	}
}

// Sender-gate tracer: a campaign_lead due for sending, plus an
// unsubscribes row for its (tenant, email) pair, must result in a
// 'skipped' email_event and a 'suppressed' campaign_lead status —
// never a 'sent' or 'bounced' event. Proves the gate is wired into
// processQueue and never reaches the SMTP path.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/worker/...
func TestProcessQueue_SuppressionGate(t *testing.T) {
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
	// Register via t.Cleanup, not defer — t.Cleanup runs in LIFO
	// order AFTER the fixture's cleanup, so the pool is still open
	// while DELETE statements execute. A bare defer would close the
	// pool before fixture cleanup, silently leaking rows.
	t.Cleanup(pool.Close)

	// Fixture: one tenant, one campaign with a one-step sequence, one
	// active campaign_lead due now, and an unsubscribes row that
	// should trip the gate. Email is unique per test run.
	tenantID := uuid.New()
	userID := uuid.New()
	personID := uuid.New()
	emailRowID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	leadID := uuid.New()
	email := "blocked-" + uuid.NewString() + "@example.com"
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
		tenantID, "Gate Test "+uuid.NewString())
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Blocked Lead", "blocked lead", "Blocked", "Lead")
	mustExec(`INSERT INTO emails (id, email, person_id) VALUES ($1, $2, $3)`,
		emailRowID, email, personID)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['email']::text[])`,
		playID, tenantID, "Gate Test Play")
	seq, _ := json.Marshal([]SequenceStep{{Step: 1, DelayDays: 1, Subject: "hello", Body: "test"}})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, sequence) VALUES ($1, $2, $3, $4, 'active', $5::jsonb)`,
		campaignID, tenantID, playID, "Gate Test Campaign", seq)
	// next_send_at in the past so GetDueLeads picks it up.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, next_send_at) VALUES ($1, $2, $3, 'active', 0, NOW() - INTERVAL '1 minute')`,
		leadID, campaignID, personID)
	// The gate condition: tenant-scoped unsubscribe for this email.
	mustExec(`INSERT INTO unsubscribes (tenant_id, email, reason) VALUES ($1, $2, $3)`,
		tenantID, email, suppression.ReasonManual)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM email_events WHERE campaign_lead_id = $1`, leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM emails WHERE id = $1`, emailRowID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM unsubscribes WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})

	// Suppression must trip before the send fn is ever invoked.
	sendCalls := 0
	send := func(ctx context.Context, _ gmail.ConnectedAccount, _ gmail.Message) (*gmail.SendResult, error) {
		sendCalls++
		return &gmail.SendResult{MessageID: "should-not-happen"}, nil
	}
	processQueue(ctx, repository.New(pool), suppression.New(pool), send, testUnsubCfg())
	if sendCalls != 0 {
		t.Errorf("send invoked despite suppression: calls=%d", sendCalls)
	}

	// One 'skipped' event with the suppression reason, no 'sent' or
	// 'bounced'.
	var skippedCount, sentCount, bouncedCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'skipped'),
			COUNT(*) FILTER (WHERE event_type = 'sent'),
			COUNT(*) FILTER (WHERE event_type = 'bounced')
		FROM email_events WHERE campaign_lead_id = $1
	`, leadID).Scan(&skippedCount, &sentCount, &bouncedCount); err != nil {
		t.Fatalf("scan event counts: %v", err)
	}
	if skippedCount != 1 {
		t.Errorf("skipped events=%d want 1", skippedCount)
	}
	if sentCount != 0 || bouncedCount != 0 {
		t.Errorf("gate let send through: sent=%d bounced=%d", sentCount, bouncedCount)
	}

	// Reason captured on the skipped event.
	var meta []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM email_events WHERE campaign_lead_id = $1 AND event_type = 'skipped'`, leadID).Scan(&meta); err != nil {
		t.Fatalf("scan skipped event: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(meta, &parsed); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if parsed["reason"] != suppression.ReasonManual {
		t.Errorf("skipped reason=%q want %q", parsed["reason"], suppression.ReasonManual)
	}

	// Lead status flipped to 'suppressed' so the next tick doesn't
	// retry the same dead send.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "suppressed" {
		t.Errorf("campaign_leads.status=%q want 'suppressed'", status)
	}
}

// sendableFixture sets up the full graph a Gmail send needs:
// tenant + user + person + email + play + campaign + active
// campaign_lead due now + a connected Gmail account. Returns the
// leadID and a teardown for the cleanup phase.
type sendableFixture struct {
	tenantID       uuid.UUID
	leadID         uuid.UUID
	gmailAccountID uuid.UUID
	email          string
}

func newSendableFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) sendableFixture {
	t.Helper()
	f := sendableFixture{
		tenantID:       uuid.New(),
		leadID:         uuid.New(),
		gmailAccountID: uuid.New(),
		email:          "lead-" + uuid.NewString() + "@example.com",
	}
	userID := uuid.New()
	personID := uuid.New()
	emailRowID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
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
		f.tenantID, "Send Test "+uuid.NewString())
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Live Lead", "live lead", "Live", "Lead")
	mustExec(`INSERT INTO emails (id, email, person_id) VALUES ($1, $2, $3)`,
		emailRowID, f.email, personID)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['email']::text[])`,
		playID, f.tenantID, "Send Test Play")
	seq, _ := json.Marshal([]SequenceStep{{Step: 1, DelayDays: 1, Subject: "hi {{first_name}}", Body: "test body"}})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, sequence) VALUES ($1, $2, $3, $4, 'active', $5::jsonb)`,
		campaignID, f.tenantID, playID, "Send Test Campaign", seq)
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, next_send_at) VALUES ($1, $2, $3, 'active', 0, NOW() - INTERVAL '1 minute')`,
		f.leadID, campaignID, personID)
	mustExec(`INSERT INTO gmail_accounts (id, tenant_id, user_id, email, access_token, refresh_token, token_expiry) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		f.gmailAccountID, f.tenantID, userID, "operator@example.com", "at", "rt", time.Now().Add(time.Hour))

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM email_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM emails WHERE id = $1`, emailRowID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM gmail_accounts WHERE id = $1`, f.gmailAccountID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// TestProcessQueue_AttachesUnsubscribeHeaders proves issue #6's
// invariant: every outbound message carries the two RFC 8058 headers.
// Inspecting the Message handed to SendFunc is enough — we already
// trust gmail.buildRFC5322 to copy Headers verbatim via its own test.
func TestProcessQueue_AttachesUnsubscribeHeaders(t *testing.T) {
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

	f := newSendableFixture(t, ctx, pool)

	var captured gmail.Message
	send := func(_ context.Context, _ gmail.ConnectedAccount, m gmail.Message) (*gmail.SendResult, error) {
		captured = m
		return &gmail.SendResult{MessageID: "<msg@x>"}, nil
	}

	processQueue(ctx, repository.New(pool), suppression.New(pool), send, testUnsubCfg())

	if captured.Headers["List-Unsubscribe-Post"] != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe-Post=%q want %q",
			captured.Headers["List-Unsubscribe-Post"], "List-Unsubscribe=One-Click")
	}
	listUnsub := captured.Headers["List-Unsubscribe"]
	if listUnsub == "" {
		t.Fatalf("missing List-Unsubscribe header")
	}
	// Both the mailto and https URLs reference the same token — the
	// recipient's email must appear in the decoded claims via the
	// public unsubscribe endpoint test, not here. Here we just spot-
	// check that the header points at the right mail-domain and api.
	if !strings.Contains(listUnsub, "@mail.test>") {
		t.Errorf("List-Unsubscribe missing test mail-domain: %q", listUnsub)
	}
	if !strings.Contains(listUnsub, "<http://api.test/api/v1/public/unsubscribe?token=") {
		t.Errorf("List-Unsubscribe missing test app URL: %q", listUnsub)
	}
	_ = f // fixture set up the leads; cleanup runs via t.Cleanup.
}

// TestProcessQueue_HappySend covers the green path: a due lead with
// no suppression and a connected Gmail account → SendFunc invoked →
// 'sent' email_event written with the returned Gmail message-id.
func TestProcessQueue_HappySend(t *testing.T) {
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
	// Register via t.Cleanup, not defer — t.Cleanup runs in LIFO
	// order AFTER the fixture's cleanup, so the pool is still open
	// while DELETE statements execute. A bare defer would close the
	// pool before fixture cleanup, silently leaking rows.
	t.Cleanup(pool.Close)

	f := newSendableFixture(t, ctx, pool)

	const wantMsgID = "<gmail-msg-happy-1@mail.google.com>"
	var sentTo string
	var sentSubject string
	send := func(_ context.Context, _ gmail.ConnectedAccount, m gmail.Message) (*gmail.SendResult, error) {
		sentTo = m.To
		sentSubject = m.Subject
		return &gmail.SendResult{MessageID: wantMsgID, ThreadID: "<thread-happy-1@mail.google.com>"}, nil
	}

	processQueue(ctx, repository.New(pool), suppression.New(pool), send, testUnsubCfg())

	if sentTo != f.email {
		t.Errorf("send To=%q want %q", sentTo, f.email)
	}
	// Personalisation must have run — {{first_name}} → "Live".
	if !strings.Contains(sentSubject, "Live") {
		t.Errorf("subject=%q missing personalised first name", sentSubject)
	}

	var sentCount int
	var msgID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE event_type='sent') FROM email_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&sentCount); err != nil {
		t.Fatalf("count sent: %v", err)
	}
	if sentCount != 1 {
		t.Errorf("sent events=%d want 1", sentCount)
	}
	if err := pool.QueryRow(ctx, `SELECT gmail_message_id FROM email_events WHERE campaign_lead_id = $1 AND event_type = 'sent'`, f.leadID).Scan(&msgID); err != nil {
		t.Fatalf("scan gmail_message_id: %v", err)
	}
	if !msgID.Valid || msgID.String != wantMsgID {
		t.Errorf("gmail_message_id=%+v want %q", msgID, wantMsgID)
	}

	// Lead's current_step advanced past 0 — exhausted (one-step seq) or active.
	var step int32
	var status string
	if err := pool.QueryRow(ctx, `SELECT current_step, status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&step, &status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if step != 1 {
		t.Errorf("current_step=%d want 1", step)
	}
	if status != "exhausted" {
		t.Errorf("status=%q want exhausted (one-step seq finished)", status)
	}
}

// TestProcessQueue_SendFailure covers the red path: SendFunc returns
// a classified gmail error → 'failed' email_event written with the
// reason on metadata. Lead step does NOT advance (so the lead retries
// next tick — except for terminal errors, which we don't model in
// this slice; see issue #5 for bounce handling).
func TestProcessQueue_SendFailure(t *testing.T) {
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
	// Register via t.Cleanup, not defer — t.Cleanup runs in LIFO
	// order AFTER the fixture's cleanup, so the pool is still open
	// while DELETE statements execute. A bare defer would close the
	// pool before fixture cleanup, silently leaking rows.
	t.Cleanup(pool.Close)

	f := newSendableFixture(t, ctx, pool)

	send := func(_ context.Context, _ gmail.ConnectedAccount, _ gmail.Message) (*gmail.SendResult, error) {
		return nil, fmt.Errorf("%w: invalid To header", gmail.ErrMessageRejected)
	}

	processQueue(ctx, repository.New(pool), suppression.New(pool), send, testUnsubCfg())

	var failedCount, sentCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'failed'),
			COUNT(*) FILTER (WHERE event_type = 'sent')
		FROM email_events WHERE campaign_lead_id = $1
	`, f.leadID).Scan(&failedCount, &sentCount); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if failedCount != 1 {
		t.Errorf("failed events=%d want 1", failedCount)
	}
	if sentCount != 0 {
		t.Errorf("sent events=%d want 0", sentCount)
	}

	var meta []byte
	if err := pool.QueryRow(ctx, `SELECT metadata FROM email_events WHERE campaign_lead_id = $1 AND event_type = 'failed'`, f.leadID).Scan(&meta); err != nil {
		t.Fatalf("scan failed event metadata: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(meta, &parsed); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if parsed["reason"] != "rejected" {
		t.Errorf("failed reason=%q want 'rejected'", parsed["reason"])
	}
}
