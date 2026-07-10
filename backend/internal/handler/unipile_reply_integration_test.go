//go:build integration

package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// replyFixture is the post-first-DM graph issue #6's inbound-message
// webhook acts on: a LinkedIn campaign_lead that is 'active' mid-sequence
// with a chat already bound (linkedin_chat_id) — the state the first DM
// (issue #5) leaves behind. The inbound-message webhook matches on that
// chat id and halts the sequence.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/handler/...
type replyFixture struct {
	tenantID uuid.UUID
	personID uuid.UUID
	leadID   uuid.UUID
	chatID   string
}

func newReplyFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) replyFixture {
	t.Helper()
	run := uuid.NewString()
	f := replyFixture{
		tenantID: uuid.New(),
		personID: uuid.New(),
		leadID:   uuid.New(),
		chatID:   "chat_" + run,
	}
	userID := uuid.New()
	identID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	accountID := uuid.New()
	clerkID := "user_test_" + run

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	mustExec(`INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		userID, clerkID, "owner-"+run+"@example.com")
	mustExec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "Reply Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		f.personID, "Grace Hopper", "grace hopper", "Grace", "Hopper")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		identID, f.personID, "https://www.linkedin.com/in/grace-"+run)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "Reply Test Play")
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel) VALUES ($1, $2, $3, $4, 'active', 'linkedin')`,
		campaignID, f.tenantID, playID, "Reply Test Campaign")
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		accountID, f.tenantID, "acc_unipile_"+run)
	// Active mid-sequence with a chat bound — the state after the first DM.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, linkedin_chat_id, accepted_at)
		VALUES ($1, $2, $3, 'active', 1, $4, $5, NOW())`,
		f.leadID, campaignID, f.personID, accountID, f.chatID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM unsubscribes WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE id = $1`, identID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, f.personID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// Direction-bearing member ids for the reply tests. The member-id
// cross-check (spike finding D / §9) is the authoritative direction signal:
// a message whose sender member id equals the connected account's own member
// id is our own outbound DM echoed back; anything else is the prospect.
const (
	ownMemberID      = "ACoAA-own-0001"
	prospectMemberID = "ACoAA-prospect-0001"
)

// signedMessageWebhook builds an authenticated message_received request in
// the real webhook shape (spike §9). The direction-bearing fields are the
// sender's member id (sender.attendee_provider_id) and the connected
// account's own member id (account_info.user_id); is_sender is carried too so
// callers can set the two signals to *disagree* and prove the member-id
// cross-check — not the presumed is_sender bool — decides direction. Auth is
// the static Unipile-Auth header.
func signedMessageWebhook(t *testing.T, chatID, messageID, senderMemberID, accountMemberID string, isSender bool) *http.Request {
	t.Helper()
	body := fmt.Sprintf(
		`{"event":"message_received","account_id":"acc_unipile","chat_id":%q,"message_id":%q,`+
			`"is_sender":%t,"account_info":{"type":"LINKEDIN","feature":"classic","user_id":%q},`+
			`"sender":{"attendee_provider_id":%q,"attendee_name":"Grace Hopper"}}`,
		chatID, messageID, isSender, accountMemberID, senderMemberID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(body))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	return req
}

// signedReplyWebhook is a real inbound prospect reply (spike §9): the sender
// member id (prospect) differs from the account's own member id, so the
// member-id cross-check classifies it inbound and the sequence halts.
func signedReplyWebhook(t *testing.T, _ *unipile.Module, chatID, messageID string) *http.Request {
	return signedMessageWebhook(t, chatID, messageID, prospectMemberID, ownMemberID, false)
}

func replyHandler(t *testing.T, pool *pgxpool.Pool, secret []byte) (*UnipileHandler, *unipile.Module) {
	t.Helper()
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret, reply: suppression.New(pool)}
	return h, mod
}

// TestUnipile_Webhook_InboundMessage_HaltsAndSuppresses is issue #6's
// reply tracer through every layer: a signed inbound-message webhook is
// verified, parsed, matched to the lead by its bound chat id, and the
// sequence halts — a 'replied' linkedin_event lands, the lead flips to
// 'replied', and a person-keyed unsubscribes row is inserted so no further
// DM ever targets the prospect. Asserts via DB.
func TestUnipile_Webhook_InboundMessage_HaltsAndSuppresses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newReplyFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	h, mod := replyHandler(t, pool, secret)
	const inboundMsgID = "msg_inbound_1"
	rr := httptest.NewRecorder()
	h.Webhook(rr, signedReplyWebhook(t, mod, f.chatID, inboundMsgID))

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	// Lead halted: status flipped to 'replied'.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "replied" {
		t.Errorf("status=%q want 'replied'", status)
	}

	// 'replied' linkedin_event written once carrying the inbound message id.
	var repliedCount int
	var gotMsgID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT COUNT(*), MAX(unipile_message_id) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'replied'`, f.leadID).
		Scan(&repliedCount, &gotMsgID); err != nil {
		t.Fatalf("scan replied event: %v", err)
	}
	if repliedCount != 1 {
		t.Errorf("replied events=%d want 1", repliedCount)
	}
	if !gotMsgID.Valid || gotMsgID.String != inboundMsgID {
		t.Errorf("replied event unipile_message_id=%+v want %q", gotMsgID, inboundMsgID)
	}

	// Person-keyed suppression row inserted with reason='reply'.
	var unsubReason string
	if err := pool.QueryRow(ctx, `SELECT reason FROM unsubscribes WHERE tenant_id = $1 AND person_id = $2`, f.tenantID, f.personID).Scan(&unsubReason); err != nil {
		t.Fatalf("scan unsubscribes: %v", err)
	}
	if unsubReason != suppression.ReasonReply {
		t.Errorf("unsubscribes.reason=%q want %q", unsubReason, suppression.ReasonReply)
	}
}

// TestUnipile_Webhook_OwnMessage_DoesNotHalt drives the connected account's
// own outbound DM, echoed back by Unipile as a message_received. Its sender
// member id equals the account's own member id (account_info.user_id), so the
// member-id cross-check classifies it as self and the sequence is NOT halted.
// is_sender is deliberately false here — the adversarial case that pins the
// self-halt fix: pre-fix, direction==is_sender==false would have wrongly
// matched the lead and halted it. The lead must stay active with no 'replied'
// event and no suppression.
func TestUnipile_Webhook_OwnMessage_DoesNotHalt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newReplyFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	h, _ := replyHandler(t, pool, secret)
	rr := httptest.NewRecorder()
	// sender == account (own member id) but is_sender:false — member id wins.
	h.Webhook(rr, signedMessageWebhook(t, f.chatID, "msg_own_1", ownMemberID, ownMemberID, false))

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	// Lead untouched — our own message must never halt the sequence.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want 'active' (own message must not halt)", status)
	}

	// No 'replied' event and no suppression row were written.
	var repliedCount, unsubCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'replied'`, f.leadID).Scan(&repliedCount); err != nil {
		t.Fatalf("scan replied count: %v", err)
	}
	if repliedCount != 0 {
		t.Errorf("replied events=%d want 0 (own message)", repliedCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM unsubscribes WHERE tenant_id = $1 AND person_id = $2`, f.tenantID, f.personID).Scan(&unsubCount); err != nil {
		t.Fatalf("scan unsub count: %v", err)
	}
	if unsubCount != 0 {
		t.Errorf("unsubscribes rows=%d want 0 (own message)", unsubCount)
	}
}

// TestUnipile_Webhook_InboundMessage_Idempotent — replaying the same
// inbound message (or a webhook the reconcile poll also caught) halts the
// lead once: one 'replied' event, one suppression row.
func TestUnipile_Webhook_InboundMessage_Idempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newReplyFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	h, mod := replyHandler(t, pool, secret)
	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.Webhook(rr, signedReplyWebhook(t, mod, f.chatID, "msg_inbound_1"))
		if rr.Code != http.StatusOK {
			t.Fatalf("delivery %d status=%d want 200", i, rr.Code)
		}
	}

	var repliedCount, unsubCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'replied'`, f.leadID).Scan(&repliedCount); err != nil {
		t.Fatalf("scan replied count: %v", err)
	}
	if repliedCount != 1 {
		t.Errorf("replied events=%d want 1 (idempotent on replay)", repliedCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM unsubscribes WHERE tenant_id = $1 AND person_id = $2`, f.tenantID, f.personID).Scan(&unsubCount); err != nil {
		t.Fatalf("scan unsub count: %v", err)
	}
	if unsubCount != 1 {
		t.Errorf("unsubscribes rows=%d want 1 (idempotent)", unsubCount)
	}
}

// TestUnipile_Webhook_InboundMessage_UnknownChat_NoOp — an inbound message
// for a chat we don't recognise matches no lead, writes nothing, and still
// returns 200 so Unipile stops retrying.
func TestUnipile_Webhook_InboundMessage_UnknownChat_NoOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newReplyFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	h, mod := replyHandler(t, pool, secret)
	rr := httptest.NewRecorder()
	h.Webhook(rr, signedReplyWebhook(t, mod, "chat_never_seen_"+uuid.NewString(), "msg_x"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (safe no-op)", rr.Code)
	}
	// The real fixture lead is untouched — still active mid-sequence.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want 'active' (unknown chat must not touch other leads)", status)
	}
}
