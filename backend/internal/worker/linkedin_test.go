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

	"github.com/jcleira/magiklead/backend/internal/linkedin/pacer"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// linkedInFixture sets up the graph a LinkedIn step-0 invite needs:
// tenant + user + person (with a linkedin_url identifier) + a LinkedIn
// campaign carrying a step-0 note + queued campaign_lead at step 0 + a
// connected, active linkedin_account. Returns the ids the assertions
// need and registers teardown.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/worker/...
type linkedInFixture struct {
	tenantID     uuid.UUID
	leadID       uuid.UUID
	campaignID   uuid.UUID
	accountID    uuid.UUID
	unipileID    string
	invitationID string
	profileURL   string
	firstName    string
}

func newLinkedInInviteFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) linkedInFixture {
	t.Helper()
	run := uuid.NewString()
	f := linkedInFixture{
		tenantID:   uuid.New(),
		leadID:     uuid.New(),
		accountID:  uuid.New(),
		unipileID:  "acc_unipile_" + run,
		profileURL: "https://www.linkedin.com/in/ada-" + run,
		firstName:  "Ada",
	}
	userID := uuid.New()
	personID := uuid.New()
	identID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	f.campaignID = campaignID
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
		f.tenantID, "LinkedIn Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Ada Lovelace", "ada lovelace", f.firstName, "Lovelace")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		identID, personID, f.profileURL)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "LinkedIn Test Play")

	seq, _ := json.Marshal([]LinkedinStep{
		{Step: 0, DelayDays: 0, Body: "Hi {{first_name}}, loved your work — let's connect."},
		{Step: 1, DelayDays: 2, Body: "Thanks for connecting, {{first_name}}!"},
	})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel, linkedin_sequence) VALUES ($1, $2, $3, $4, 'active', 'linkedin', $5::jsonb)`,
		campaignID, f.tenantID, playID, "LinkedIn Test Campaign", seq)
	// Queued at step 0, no next_send_at — a freshly added prospect that
	// has not yet had its invite sent.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step) VALUES ($1, $2, $3, 'queued', 0)`,
		f.leadID, campaignID, personID)
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		f.accountID, f.tenantID, f.unipileID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, f.accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE person_id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// newLinkedInDMFixture sets up the post-acceptance graph the DM tick
// (issue #5) acts on: the same tenant/person/account shape as the invite
// fixture, but the campaign_lead is already 'active' at step 1 and due now
// (the state MarkLinkedInAccepted leaves behind), bound to the account that
// sent its invite. The campaign carries a 3-step sequence so sending step 1
// advances to step 2 rather than exhausting.
func newLinkedInDMFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) linkedInFixture {
	t.Helper()
	run := uuid.NewString()
	f := linkedInFixture{
		tenantID:   uuid.New(),
		leadID:     uuid.New(),
		accountID:  uuid.New(),
		unipileID:  "acc_unipile_" + run,
		profileURL: "https://www.linkedin.com/in/grace-" + run,
		firstName:  "Grace",
	}
	userID := uuid.New()
	personID := uuid.New()
	identID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	f.campaignID = campaignID
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
		f.tenantID, "LinkedIn DM Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Grace Hopper", "grace hopper", f.firstName, "Hopper")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		identID, personID, f.profileURL)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "LinkedIn DM Test Play")

	seq, _ := json.Marshal([]LinkedinStep{
		{Step: 0, DelayDays: 0, Body: "Hi {{first_name}}, let's connect."},
		{Step: 1, DelayDays: 2, Body: "Thanks for connecting, {{first_name}}!"},
		{Step: 2, DelayDays: 3, Body: "Following up, {{first_name}}."},
	})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel, linkedin_sequence) VALUES ($1, $2, $3, $4, 'active', 'linkedin', $5::jsonb)`,
		campaignID, f.tenantID, playID, "LinkedIn DM Test Campaign", seq)
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		f.accountID, f.tenantID, f.unipileID)
	// Accepted and due: active at step 1, next_send_at in the past, bound to
	// the account + invitation. No chat yet — the first DM starts one.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, next_send_at, linkedin_account_id, linkedin_invitation_id, accepted_at)
		VALUES ($1, $2, $3, 'active', 1, NOW() - INTERVAL '1 minute', $4, $5, NOW())`,
		f.leadID, campaignID, personID, f.accountID, "inv_"+run)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, f.accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE person_id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// TestProcessLinkedInDMQueue_HappyFirstDM is the acceptance→DM tracer at
// the worker layer: an accepted lead (active, step 1, due) gets DM step 1
// sent via the Unipile message seam from the same account that sent its
// invite, to the prospect, with the note personalised; a 'dm_sent' event
// lands carrying the returned message + chat ids; and the lead advances to
// step 2's jittered delay with the chat id recorded for the next step.
func TestProcessLinkedInDMQueue_HappyFirstDM(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInDMFixture(t, ctx, pool)
	// The invite already resolved and cached the member id; the DM reuses it.
	const wantMemberID = "ACoAAdmreuse006"
	personID := personIDForURL(t, ctx, pool, f.profileURL)
	seedMemberID(t, ctx, pool, personID, wantMemberID)

	const wantMsgID = "msg_dm_1"
	const wantChatID = "chat_dm_1"
	var captured unipile.MessageParams
	calls := 0
	send := func(_ context.Context, p unipile.MessageParams) (*unipile.MessageResult, error) {
		calls++
		captured = p
		return &unipile.MessageResult{MessageID: wantMsgID, ChatID: wantChatID}, nil
	}
	// A cached member id must be reused without a second Unipile lookup.
	resolve := func(_ context.Context, _, _ string) (string, error) {
		t.Errorf("DM resolved the member id upstream — a cached id must be reused")
		return "", nil
	}

	processLinkedInDMQueue(ctx, repository.New(pool), suppression.New(pool), send, resolve)

	// Exactly one DM, from the bound account, to the prospect, personalised.
	if calls != 1 {
		t.Fatalf("send calls=%d want 1", calls)
	}
	if captured.AccountID != f.unipileID {
		t.Errorf("dm account_id=%q want %q", captured.AccountID, f.unipileID)
	}
	if captured.Recipient != wantMemberID {
		t.Errorf("dm recipient=%q want the cached member id %q (not the profile URL)", captured.Recipient, wantMemberID)
	}
	if !strings.Contains(captured.Text, f.firstName) {
		t.Errorf("dm text=%q missing personalised first name %q", captured.Text, f.firstName)
	}
	if strings.Contains(captured.Text, "{{first_name}}") {
		t.Errorf("dm text=%q still has an unrendered token", captured.Text)
	}

	// dm_sent event at step 1 carrying the returned message + chat ids.
	var sentCount int
	var msgID, chatID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE event_type='dm_sent') FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&sentCount); err != nil {
		t.Fatalf("count dm_sent: %v", err)
	}
	if sentCount != 1 {
		t.Errorf("dm_sent events=%d want 1", sentCount)
	}
	if err := pool.QueryRow(ctx, `SELECT unipile_message_id, unipile_chat_id FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type='dm_sent'`, f.leadID).Scan(&msgID, &chatID); err != nil {
		t.Fatalf("scan dm_sent ids: %v", err)
	}
	if !msgID.Valid || msgID.String != wantMsgID {
		t.Errorf("dm_sent unipile_message_id=%+v want %q", msgID, wantMsgID)
	}
	if !chatID.Valid || chatID.String != wantChatID {
		t.Errorf("dm_sent unipile_chat_id=%+v want %q", chatID, wantChatID)
	}

	// Lead advanced to step 2 with a future schedule, still active, chat bound.
	var status string
	var step int32
	var nextSendAt pgtype.Timestamptz
	var leadChatID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT status, current_step, next_send_at, linkedin_chat_id FROM campaign_leads WHERE id = $1`, f.leadID).
		Scan(&status, &step, &nextSendAt, &leadChatID); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want active (more steps remain)", status)
	}
	if step != 2 {
		t.Errorf("current_step=%d want 2", step)
	}
	if !nextSendAt.Valid || !nextSendAt.Time.After(time.Now()) {
		t.Errorf("next_send_at=%v want a future time (step 2's delay)", nextSendAt)
	}
	if !leadChatID.Valid || leadChatID.String != wantChatID {
		t.Errorf("lead linkedin_chat_id=%+v want %q (recorded for step 2)", leadChatID, wantChatID)
	}
}

// TestProcessLinkedInDMQueue_AdvancesToExhaustion drives the multi-step DM
// path (issue #6): a lead mid-sequence advances on its follow-up steps and
// ends 'exhausted' after the last one. Two ticks (step 1 then step 2 of a
// 3-step sequence) take it from active to exhausted with two dm_sent events.
func TestProcessLinkedInDMQueue_AdvancesToExhaustion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInDMFixture(t, ctx, pool) // 3-step seq, lead active at step 1, due
	// The invite cached the member id; every follow-up step reuses it.
	seedMemberID(t, ctx, pool, personIDForURL(t, ctx, pool, f.profileURL), "ACoAAdmfollowup6")

	send := func(_ context.Context, _ unipile.MessageParams) (*unipile.MessageResult, error) {
		return &unipile.MessageResult{MessageID: "msg_" + uuid.NewString(), ChatID: "chat_exhaust"}, nil
	}
	resolve := func(_ context.Context, _, _ string) (string, error) {
		t.Errorf("follow-up DM resolved the member id upstream — the cached id must be reused")
		return "", nil
	}
	q := repository.New(pool)
	supp := suppression.New(pool)

	// Tick 1: send step 1, advance to step 2 (scheduled in the future).
	processLinkedInDMQueue(ctx, q, supp, send, resolve)
	var stepAfter1 int32
	var statusAfter1 string
	if err := pool.QueryRow(ctx, `SELECT current_step, status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&stepAfter1, &statusAfter1); err != nil {
		t.Fatalf("scan after tick 1: %v", err)
	}
	if stepAfter1 != 2 || statusAfter1 != "active" {
		t.Fatalf("after tick 1 step=%d status=%q want 2/active", stepAfter1, statusAfter1)
	}

	// Make step 2 due, then tick again: send step 2, nextStep=3 == len → exhausted.
	if _, err := pool.Exec(ctx, `UPDATE campaign_leads SET next_send_at = NOW() - INTERVAL '1 minute' WHERE id = $1`, f.leadID); err != nil {
		t.Fatalf("force step 2 due: %v", err)
	}
	processLinkedInDMQueue(ctx, q, supp, send, resolve)

	var status string
	var step int32
	var nextSendAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT status, current_step, next_send_at FROM campaign_leads WHERE id = $1`, f.leadID).
		Scan(&status, &step, &nextSendAt); err != nil {
		t.Fatalf("scan after tick 2: %v", err)
	}
	if status != "exhausted" {
		t.Errorf("status=%q want exhausted (sequence ran out)", status)
	}
	if step != 3 {
		t.Errorf("current_step=%d want 3 (advanced past the last step)", step)
	}
	if nextSendAt.Valid {
		t.Errorf("next_send_at=%v want NULL once exhausted", nextSendAt.Time)
	}

	// Both DMs fired — one per step.
	var sent int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'dm_sent'`, f.leadID).Scan(&sent); err != nil {
		t.Fatalf("count dm_sent: %v", err)
	}
	if sent != 2 {
		t.Errorf("dm_sent events=%d want 2 (steps 1 and 2)", sent)
	}
}

// TestProcessLinkedInDMQueue_SuppressedPersonSkipped proves the zero-DMs-
// to-replied invariant (issue #6): a lead that is genuinely active + due
// but whose *person* is suppressed (e.g. they replied via another
// campaign_lead) is never messaged. The tick writes a 'skipped' event and
// halts the lead at 'suppressed'; the send seam is never called.
func TestProcessLinkedInDMQueue_SuppressedPersonSkipped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInDMFixture(t, ctx, pool)

	// Suppress the person directly (leaving THIS lead active + due) so the
	// step is genuinely due when the gate catches it.
	if _, err := pool.Exec(ctx, `
		INSERT INTO unsubscribes (tenant_id, person_id, reason)
		SELECT c.tenant_id, cl.person_id, 'reply'
		FROM campaign_leads cl JOIN campaigns c ON c.id = cl.campaign_id
		WHERE cl.id = $1`, f.leadID); err != nil {
		t.Fatalf("seed person suppression: %v", err)
	}

	calls := 0
	send := func(_ context.Context, _ unipile.MessageParams) (*unipile.MessageResult, error) {
		calls++
		return &unipile.MessageResult{MessageID: "should-not-send", ChatID: "x"}, nil
	}

	processLinkedInDMQueue(ctx, repository.New(pool), suppression.New(pool), send, resolveOK)

	// No DM sent.
	if calls != 0 {
		t.Errorf("send invoked for a suppressed person: calls=%d", calls)
	}

	// One 'skipped' event, no 'dm_sent'.
	var skipped, sent int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type = 'skipped'),
		       COUNT(*) FILTER (WHERE event_type = 'dm_sent')
		FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&skipped, &sent); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if skipped != 1 {
		t.Errorf("skipped events=%d want 1", skipped)
	}
	if sent != 0 {
		t.Errorf("dm_sent events=%d want 0", sent)
	}

	// Lead halted at 'suppressed' — drops out of the due query next tick.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "suppressed" {
		t.Errorf("status=%q want 'suppressed'", status)
	}
}

// TestProcessLinkedInDMQueue_SkipsRestrictedAccount proves issue #8's pause of
// in-flight work: an accepted, active, due DM lead bound to an account LinkedIn
// has since restricted is not messaged — the account-health filter on
// GetDueLinkedInDMLeads drops it. The send seam is never called, no event is
// written, and the lead stays active so it resumes once the account reconnects.
func TestProcessLinkedInDMQueue_SkipsRestrictedAccount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInDMFixture(t, ctx, pool)

	// LinkedIn restricted the account after the invite was accepted — its
	// in-flight DM leads must pause until it is healthy again.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'restricted', last_error = 'ERROR' WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("set account restricted: %v", err)
	}

	calls := 0
	send := func(_ context.Context, _ unipile.MessageParams) (*unipile.MessageResult, error) {
		calls++
		return &unipile.MessageResult{MessageID: "should-not-send", ChatID: "x"}, nil
	}

	processLinkedInDMQueue(ctx, repository.New(pool), suppression.New(pool), send, resolveOK)

	if calls != 0 {
		t.Errorf("send invoked for a restricted account: calls=%d", calls)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Errorf("linkedin_events=%d want 0 (nothing sent while restricted)", events)
	}
	// Lead untouched: still active at step 1, so it resumes on reconnect.
	var status string
	var step int32
	if err := pool.QueryRow(ctx, `SELECT status, current_step FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status, &step); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" || step != 1 {
		t.Errorf("lead status=%q step=%d want active/1 (paused in place, not advanced)", status, step)
	}
}

// newLinkedInStaleInviteFixture sets up a lead parked awaiting acceptance
// whose invite was sent inviteAge ago — the shape the withdrawal sweep
// (issue #7) acts on: a LinkedIn campaign_lead at awaiting_accept, bound to
// the account that sent its invite and to a Unipile invitation id, with
// last_sent_at (the invite-sent time for a parked lead) set inviteAge in the
// past. Pass an age over 21 days to make it withdrawable, under to keep it
// parked.
func newLinkedInStaleInviteFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, inviteAge time.Duration) linkedInFixture {
	t.Helper()
	run := uuid.NewString()
	f := linkedInFixture{
		tenantID:     uuid.New(),
		leadID:       uuid.New(),
		accountID:    uuid.New(),
		unipileID:    "acc_unipile_" + run,
		invitationID: "inv_stale_" + run,
		profileURL:   "https://www.linkedin.com/in/alan-" + run,
		firstName:    "Alan",
	}
	userID := uuid.New()
	personID := uuid.New()
	identID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
	f.campaignID = campaignID
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
		f.tenantID, "LinkedIn Stale Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Alan Turing", "alan turing", f.firstName, "Turing")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		identID, personID, f.profileURL)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "LinkedIn Stale Test Play")

	seq, _ := json.Marshal([]LinkedinStep{{Step: 0, DelayDays: 0, Body: "Hi {{first_name}}, let's connect."}})
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel, linkedin_sequence) VALUES ($1, $2, $3, $4, 'active', 'linkedin', $5::jsonb)`,
		campaignID, f.tenantID, playID, "LinkedIn Stale Test Campaign", seq)
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		f.accountID, f.tenantID, f.unipileID)
	// Parked awaiting acceptance, bound to the account + invitation, invite
	// sent inviteAge ago via last_sent_at.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, linkedin_invitation_id, last_sent_at)
		VALUES ($1, $2, $3, 'awaiting_accept', 0, $4, $5, NOW() - $6::interval)`,
		f.leadID, campaignID, personID, f.accountID, f.invitationID, fmt.Sprintf("%d seconds", int(inviteAge.Seconds())))

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, f.accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE person_id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// TestProcessLinkedInWithdrawals_WithdrawsStaleInvite is AC3's tracer: a lead
// parked awaiting_accept whose invite is older than the 21-day horizon is
// cancelled at Unipile (asserted against the stub), a 'withdrawn' event lands
// carrying the invitation id, and the lead is closed 'not_accepted'.
func TestProcessLinkedInWithdrawals_WithdrawsStaleInvite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInStaleInviteFixture(t, ctx, pool, 22*24*time.Hour)

	var gotAccount, gotInvitation string
	calls := 0
	cancelInvite := func(_ context.Context, accountID, invitationID string) error {
		calls++
		gotAccount, gotInvitation = accountID, invitationID
		return nil
	}

	processLinkedInWithdrawals(ctx, repository.New(pool), cancelInvite)

	// Cancelled exactly once, from the bound account, for the bound invitation.
	if calls != 1 {
		t.Fatalf("cancel calls=%d want 1", calls)
	}
	if gotAccount != f.unipileID {
		t.Errorf("cancel account=%q want %q", gotAccount, f.unipileID)
	}
	if gotInvitation != f.invitationID {
		t.Errorf("cancel invitation=%q want %q", gotInvitation, f.invitationID)
	}

	// A 'withdrawn' event written, carrying the invitation id.
	var withdrawn int
	var msgID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE event_type='withdrawn') FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID).Scan(&withdrawn); err != nil {
		t.Fatalf("count withdrawn: %v", err)
	}
	if withdrawn != 1 {
		t.Errorf("withdrawn events=%d want 1", withdrawn)
	}
	if err := pool.QueryRow(ctx, `SELECT unipile_message_id FROM linkedin_events WHERE campaign_lead_id=$1 AND event_type='withdrawn'`, f.leadID).Scan(&msgID); err != nil {
		t.Fatalf("scan withdrawn invitation id: %v", err)
	}
	if !msgID.Valid || msgID.String != f.invitationID {
		t.Errorf("withdrawn unipile_message_id=%+v want %q", msgID, f.invitationID)
	}

	// Lead closed not_accepted.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "not_accepted" {
		t.Errorf("status=%q want not_accepted", status)
	}
}

// TestProcessLinkedInWithdrawals_LeavesFreshInvite proves the 21-day
// boundary: a parked invite only 5 days old is left untouched — not
// cancelled, no event, still awaiting_accept.
func TestProcessLinkedInWithdrawals_LeavesFreshInvite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInStaleInviteFixture(t, ctx, pool, 5*24*time.Hour)

	calls := 0
	cancelInvite := func(_ context.Context, _, _ string) error {
		calls++
		return nil
	}

	processLinkedInWithdrawals(ctx, repository.New(pool), cancelInvite)

	if calls != 0 {
		t.Errorf("cancel invoked for a fresh invite: calls=%d", calls)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Errorf("events=%d want 0 (nothing withdrawn)", events)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept (untouched)", status)
	}
}

// TestProcessLinkedInWithdrawals_CancelFailureLeavesLead proves we never
// close a lead whose invite is still live at LinkedIn: when the Unipile
// cancel errors, the lead stays awaiting_accept and no 'withdrawn' event is
// written, so the next sweep retries it.
func TestProcessLinkedInWithdrawals_CancelFailureLeavesLead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInStaleInviteFixture(t, ctx, pool, 22*24*time.Hour)

	cancelInvite := func(_ context.Context, _, _ string) error {
		return unipile.ErrRateLimited
	}

	processLinkedInWithdrawals(ctx, repository.New(pool), cancelInvite)

	var withdrawn int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE event_type='withdrawn') FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID).Scan(&withdrawn); err != nil {
		t.Fatalf("count withdrawn: %v", err)
	}
	if withdrawn != 0 {
		t.Errorf("withdrawn events=%d want 0 (cancel failed)", withdrawn)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan status: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept (not closed on cancel failure)", status)
	}
}

func linkedInTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	// t.Cleanup (not defer) so the pool outlives the fixture's own
	// cleanup, which runs first under LIFO.
	t.Cleanup(pool.Close)
	return pool
}

// seedMemberID caches memberID as the person's linkedin_member_id identifier —
// the state a prior invite's resolve leaves behind — so the send path finds it
// on a cache hit and never calls Unipile. The fixture's teardown (which deletes
// every person_identifiers row for the person) removes it.
func seedMemberID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, personID uuid.UUID, memberID string) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_member_id', $3, FALSE)`,
		uuid.New(), personID, memberID); err != nil {
		t.Fatalf("seed linkedin_member_id: %v", err)
	}
}

// resolveOK is a resolve seam that always yields a fixed member id, for the
// send-path tests where the resolve is not itself under test but must succeed
// so the send is reached. Any member id it persists is torn down by the fixture.
func resolveOK(_ context.Context, _, _ string) (string, error) {
	return "ACoAAtestresolve01", nil
}

// TestProcessLinkedInQueue_HappyInvite is the tracer through every layer:
// a queued LinkedIn lead → the tick resolves the prospect's Unipile member
// id (a cache miss, so Unipile is called once and the id persisted on the
// person), asks the pacer, renders the step-0 note, calls the Unipile send
// seam addressed by the resolved member id (not the profile URL), writes an
// invite_sent event, parks the lead awaiting_accept bound to the account +
// invitation, and increments the account's rolling counters. Covers the
// resolve-then-send-by-member-id AC and the "invite comes from the connected
// account" AC.
func TestProcessLinkedInQueue_HappyInvite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)
	personID := personIDForURL(t, ctx, pool, f.profileURL)

	const wantInvitationID = "inv_happy_1"
	const wantMemberID = "ACoAAhappyinvite6"
	var captured unipile.InviteParams
	calls := 0
	send := func(_ context.Context, p unipile.InviteParams) (*unipile.InviteResult, error) {
		calls++
		captured = p
		return &unipile.InviteResult{InvitationID: wantInvitationID}, nil
	}
	// Cache miss: the resolver calls Unipile once with the prospect's URL,
	// through the connected account, and hands back the member id.
	resolveCalls := 0
	resolve := func(_ context.Context, profileURL, accountID string) (string, error) {
		resolveCalls++
		if profileURL != f.profileURL {
			t.Errorf("resolve profileURL=%q want %q", profileURL, f.profileURL)
		}
		if accountID != f.unipileID {
			t.Errorf("resolve accountID=%q want %q", accountID, f.unipileID)
		}
		return wantMemberID, nil
	}

	processLinkedInQueue(ctx, repository.New(pool), send, resolve, pacer.Standard())

	// The member id was resolved exactly once and the invite addressed by it,
	// from the connected account, with the note personalised.
	if calls != 1 {
		t.Fatalf("send calls=%d want 1", calls)
	}
	if resolveCalls != 1 {
		t.Fatalf("resolve calls=%d want 1 (cache miss resolves once)", resolveCalls)
	}
	if captured.AccountID != f.unipileID {
		t.Errorf("invite account_id=%q want %q", captured.AccountID, f.unipileID)
	}
	if captured.Recipient != wantMemberID {
		t.Errorf("invite recipient=%q want the resolved member id %q (not the profile URL)", captured.Recipient, wantMemberID)
	}
	if !strings.Contains(captured.Note, f.firstName) {
		t.Errorf("note=%q missing personalised first name %q", captured.Note, f.firstName)
	}
	if strings.Contains(captured.Note, "{{first_name}}") {
		t.Errorf("note=%q still has an unrendered token", captured.Note)
	}

	// The resolved member id was persisted on the person for reuse.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM person_identifiers WHERE person_id=$1 AND identifier_type='linkedin_member_id' AND identifier_value=$2`, personID, wantMemberID); n != 1 {
		t.Errorf("persisted linkedin_member_id rows=%d want 1", n)
	}

	// invite_sent event written at step 0, carrying the invitation id.
	var sentCount int
	var msgID pgtype.Text
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE event_type='invite_sent') FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&sentCount); err != nil {
		t.Fatalf("count invite_sent: %v", err)
	}
	if sentCount != 1 {
		t.Errorf("invite_sent events=%d want 1", sentCount)
	}
	if err := pool.QueryRow(ctx, `SELECT unipile_message_id FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type='invite_sent'`, f.leadID).Scan(&msgID); err != nil {
		t.Fatalf("scan unipile_message_id: %v", err)
	}
	if !msgID.Valid || msgID.String != wantInvitationID {
		t.Errorf("unipile_message_id=%+v want %q", msgID, wantInvitationID)
	}

	// Lead parked awaiting acceptance, next_send_at cleared, account +
	// invitation bound, still at step 0.
	var status string
	var nextSendAt pgtype.Timestamptz
	var acctID pgtype.UUID
	var invID pgtype.Text
	var step int32
	if err := pool.QueryRow(ctx, `SELECT status, next_send_at, linkedin_account_id, linkedin_invitation_id, current_step FROM campaign_leads WHERE id = $1`, f.leadID).
		Scan(&status, &nextSendAt, &acctID, &invID, &step); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept", status)
	}
	if nextSendAt.Valid {
		t.Errorf("next_send_at=%v want NULL", nextSendAt.Time)
	}
	if uuid.UUID(acctID.Bytes) != f.accountID {
		t.Errorf("linkedin_account_id=%x want %s", acctID.Bytes, f.accountID)
	}
	if !invID.Valid || invID.String != wantInvitationID {
		t.Errorf("linkedin_invitation_id=%+v want %q", invID, wantInvitationID)
	}
	if step != 0 {
		t.Errorf("current_step=%d want 0 (acceptance, not a timer, advances it)", step)
	}

	// Account counters incremented.
	var weekly, daily int32
	if err := pool.QueryRow(ctx, `SELECT weekly_invite_count, daily_invite_count FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&weekly, &daily); err != nil {
		t.Fatalf("scan counters: %v", err)
	}
	if weekly != 1 {
		t.Errorf("weekly_invite_count=%d want 1", weekly)
	}
	if daily != 1 {
		t.Errorf("daily_invite_count=%d want 1", daily)
	}
}

// TestProcessLinkedInQueue_PacerBudgetZero proves the pacing gate: an
// account already at its weekly ceiling (window still open) sends nothing
// this tick and the lead is left queued for a later tick. The send seam
// is never called and no counters move.
func TestProcessLinkedInQueue_PacerBudgetZero(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)

	// Drive the account to its weekly cap inside the current week.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET weekly_invite_count = $2, weekly_window_started_at = NOW() WHERE id = $1`,
		f.accountID, pacer.StandardWeeklyCap); err != nil {
		t.Fatalf("seed weekly cap: %v", err)
	}

	calls := 0
	send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
		calls++
		return &unipile.InviteResult{InvitationID: "should-not-happen"}, nil
	}

	processLinkedInQueue(ctx, repository.New(pool), send, resolveOK, pacer.Standard())

	if calls != 0 {
		t.Errorf("send invoked despite exhausted budget: calls=%d", calls)
	}

	// No invite_sent event.
	var events int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Errorf("linkedin_events=%d want 0 (nothing sent)", events)
	}

	// Lead untouched: still queued at step 0, no account binding.
	var status string
	var step int32
	var acctID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT status, current_step, linkedin_account_id FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status, &step, &acctID); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "queued" {
		t.Errorf("status=%q want queued (untouched)", status)
	}
	if step != 0 {
		t.Errorf("current_step=%d want 0", step)
	}
	if acctID.Valid {
		t.Errorf("linkedin_account_id set (%x) but no invite was sent", acctID.Bytes)
	}

	// Counter unchanged at the cap.
	var weekly int32
	if err := pool.QueryRow(ctx, `SELECT weekly_invite_count FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&weekly); err != nil {
		t.Fatalf("scan counter: %v", err)
	}
	if weekly != pacer.StandardWeeklyCap {
		t.Errorf("weekly_invite_count=%d want %d (unchanged)", weekly, pacer.StandardWeeklyCap)
	}
}

// TestProcessLinkedInQueue_SendErrorClassified proves a Unipile send
// failure is classified into a short reason on a 'failed' linkedin_event
// and the lead is NOT advanced — it stays queued at step 0 for a retry,
// the account binding is never written, and the counters never move.
// It also pins issue #8's escalation: only the restricted sentinel flips
// the account to status='restricted' (so the UI can prompt a reconnect);
// transient rate-limit and config-level auth errors leave the account
// active for the next tick to retry.
func TestProcessLinkedInQueue_SendErrorClassified(t *testing.T) {
	cases := []struct {
		name              string
		sendErr           error
		wantReason        string
		wantAccountStatus string
	}{
		{"restricted", fmt.Errorf("%w: account is restricted", unipile.ErrAccountRestricted), "restricted", "restricted"},
		{"rate_limited", fmt.Errorf("%w", unipile.ErrRateLimited), "rate_limited", "active"},
		{"auth", fmt.Errorf("%w", unipile.ErrUnauthorized), "auth", "active"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			pool := linkedInTestPool(t, ctx)
			f := newLinkedInInviteFixture(t, ctx, pool)

			send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
				return nil, tc.sendErr
			}

			processLinkedInQueue(ctx, repository.New(pool), send, resolveOK, pacer.Standard())

			// One failed event, no invite_sent.
			var failed, sent int
			if err := pool.QueryRow(ctx, `
				SELECT
					COUNT(*) FILTER (WHERE event_type = 'failed'),
					COUNT(*) FILTER (WHERE event_type = 'invite_sent')
				FROM linkedin_events WHERE campaign_lead_id = $1
			`, f.leadID).Scan(&failed, &sent); err != nil {
				t.Fatalf("count events: %v", err)
			}
			if failed != 1 {
				t.Errorf("failed events=%d want 1", failed)
			}
			if sent != 0 {
				t.Errorf("invite_sent events=%d want 0", sent)
			}

			// Reason captured on the failed event.
			var meta []byte
			if err := pool.QueryRow(ctx, `SELECT metadata FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type='failed'`, f.leadID).Scan(&meta); err != nil {
				t.Fatalf("scan failed metadata: %v", err)
			}
			var parsed map[string]string
			if err := json.Unmarshal(meta, &parsed); err != nil {
				t.Fatalf("unmarshal metadata: %v", err)
			}
			if parsed["reason"] != tc.wantReason {
				t.Errorf("failed reason=%q want %q", parsed["reason"], tc.wantReason)
			}

			// Lead not advanced.
			var status string
			var step int32
			var acctID pgtype.UUID
			if err := pool.QueryRow(ctx, `SELECT status, current_step, linkedin_account_id FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status, &step, &acctID); err != nil {
				t.Fatalf("scan lead: %v", err)
			}
			if status != "queued" {
				t.Errorf("status=%q want queued (not advanced on failure)", status)
			}
			if step != 0 {
				t.Errorf("current_step=%d want 0", step)
			}
			if acctID.Valid {
				t.Errorf("linkedin_account_id set (%x) but the send failed", acctID.Bytes)
			}

			// Counters untouched.
			var weekly int32
			if err := pool.QueryRow(ctx, `SELECT weekly_invite_count FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&weekly); err != nil {
				t.Fatalf("scan counter: %v", err)
			}
			if weekly != 0 {
				t.Errorf("weekly_invite_count=%d want 0 (no successful send)", weekly)
			}

			// Account escalation (issue #8): the restricted sentinel flips the
			// account to 'restricted' and records the error so the UI can prompt
			// a reconnect; transient/config errors leave it active.
			var accStatus string
			var lastErr pgtype.Text
			if err := pool.QueryRow(ctx, `SELECT status, last_error FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&accStatus, &lastErr); err != nil {
				t.Fatalf("scan account status: %v", err)
			}
			if accStatus != tc.wantAccountStatus {
				t.Errorf("account status=%q want %q", accStatus, tc.wantAccountStatus)
			}
			if tc.wantAccountStatus == "restricted" && !lastErr.Valid {
				t.Errorf("last_error is NULL want the send error recorded on escalation")
			}
			if tc.wantAccountStatus == "active" && lastErr.Valid {
				t.Errorf("last_error=%q set but a transient error must not escalate", lastErr.String)
			}
		})
	}
}

// TestProcessLinkedInQueue_SkipsRestrictedThenResumes proves issue #8's
// invite-tick gating both ways: while the account is restricted the tick sends
// nothing and leaves the due lead queued (pickLinkedInAccount only picks
// active/warming); once the account is flipped back to active — the state the
// reconnect webhook leaves behind — the very next tick resumes it and the
// invite goes out.
func TestProcessLinkedInQueue_SkipsRestrictedThenResumes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)

	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'restricted', last_error = 'ERROR' WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("set account restricted: %v", err)
	}

	calls := 0
	send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
		calls++
		return &unipile.InviteResult{InvitationID: "inv_resume_" + uuid.NewString()}, nil
	}
	q := repository.New(pool)

	// Tick 1: account restricted → nothing sent, lead left queued, no events.
	processLinkedInQueue(ctx, q, send, resolveOK, pacer.Standard())
	if calls != 0 {
		t.Fatalf("send invoked while account restricted: calls=%d", calls)
	}
	var status string
	var events int
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "queued" {
		t.Errorf("status=%q want queued (account restricted)", status)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 0 {
		t.Errorf("linkedin_events=%d want 0 (restricted account sends nothing)", events)
	}

	// Reconnect: the account.connected upsert lands the account back at
	// 'active'. The next tick must resume the parked lead.
	if _, err := pool.Exec(ctx, `UPDATE linkedin_accounts SET status = 'active', last_error = NULL WHERE id = $1`, f.accountID); err != nil {
		t.Fatalf("reactivate account: %v", err)
	}

	// Tick 2: account healthy → invite goes out, lead parks awaiting_accept.
	processLinkedInQueue(ctx, q, send, resolveOK, pacer.Standard())
	if calls != 1 {
		t.Fatalf("send calls=%d want 1 after reconnect", calls)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead after resume: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept (invite sent after reconnect)", status)
	}
}

// TestProcessLinkedInQueue_AcceptanceBreakerPausesAndResumes drives the
// acceptance breaker end-to-end at the worker layer (issue #7), exercising
// the trailing-7-day signal query (AC4) against real seeded events: an
// account whose history shows <20% acceptance has its invites paused — the
// due lead is left queued and acceptance_paused flips true — and once the
// rate recovers to ≥20% the account resumes and the invite goes out.
func TestProcessLinkedInQueue_AcceptanceBreakerPausesAndResumes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	// A history lead bound to the account (its own person, to satisfy the
	// campaign_leads target CHECK + the per-campaign person uniqueness index)
	// anchors the seeded invite/accept events the breaker counts. Terminal
	// status so it is never itself picked up as due for an invite.
	histPerson := uuid.New()
	histLead := uuid.New()
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name) VALUES ($1, 'Hist Person', 'hist person')`, histPerson)
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id) VALUES ($1, $2, $3, 'not_accepted', 0, $4)`,
		histLead, f.campaignID, histPerson, f.accountID)
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, histLead)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, histLead)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, histPerson)
	})

	// 30 invites sent, 3 accepted in the trailing window → 10% < the 20%
	// floor, well past the minimum sample, so the breaker must trip.
	mustExec(`INSERT INTO linkedin_events (campaign_lead_id, event_type, step, created_at)
		SELECT $1, 'invite_sent', 0, NOW() FROM generate_series(1, 30)`, histLead)
	mustExec(`INSERT INTO linkedin_events (campaign_lead_id, event_type, step, created_at)
		SELECT $1, 'accepted', 1, NOW() FROM generate_series(1, 3)`, histLead)

	calls := 0
	send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
		calls++
		return &unipile.InviteResult{InvitationID: "should-not-send-while-paused"}, nil
	}
	q := repository.New(pool)

	// Tick 1: breaker tripped → no send, account flagged paused.
	processLinkedInQueue(ctx, q, send, resolveOK, pacer.Standard())

	if calls != 0 {
		t.Fatalf("send invoked while breaker tripped: calls=%d", calls)
	}
	var paused bool
	var rate float64
	if err := pool.QueryRow(ctx, `SELECT acceptance_paused, acceptance_rate FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&paused, &rate); err != nil {
		t.Fatalf("scan account state: %v", err)
	}
	if !paused {
		t.Errorf("acceptance_paused=false want true (10%% acceptance is below the floor)")
	}
	if rate < 0.09 || rate > 0.11 {
		t.Errorf("acceptance_rate=%v want ~0.10", rate)
	}
	// The due lead is left queued — no invite event written.
	var status string
	var sentEvents int
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "queued" {
		t.Errorf("status=%q want queued (breaker paused the account)", status)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&sentEvents); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if sentEvents != 0 {
		t.Errorf("linkedin_events on the due lead=%d want 0 (nothing sent)", sentEvents)
	}

	// Recovery: 7 more acceptances → 10/30 ≈ 33% ≥ the floor.
	mustExec(`INSERT INTO linkedin_events (campaign_lead_id, event_type, step, created_at)
		SELECT $1, 'accepted', 1, NOW() FROM generate_series(1, 7)`, histLead)

	// Tick 2: breaker clears → account resumes and the invite goes out.
	processLinkedInQueue(ctx, q, send, resolveOK, pacer.Standard())

	if calls != 1 {
		t.Fatalf("send calls=%d want 1 after recovery", calls)
	}
	if err := pool.QueryRow(ctx, `SELECT acceptance_paused FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&paused); err != nil {
		t.Fatalf("scan paused: %v", err)
	}
	if paused {
		t.Errorf("acceptance_paused=true want false (rate recovered to ~33%%)")
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead after resume: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept (invite sent after resume)", status)
	}
}

// TestProcessLinkedInQueue_TransientResolveLeavesLeadQueued wires issue #5's
// transient-resolve path into the invite loop (issue #6): a member-id resolve
// that fails on transient upstream trouble (rate limit / 5xx / timeout) leaves
// the lead exactly as it was — still queued, nothing sent — and, because the
// resolve runs before the pacing gate, consumes no invite allowance. The send
// seam is never reached and no failure event is written; the next tick retries.
func TestProcessLinkedInQueue_TransientResolveLeavesLeadQueued(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)

	sendCalls := 0
	send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
		sendCalls++
		return &unipile.InviteResult{InvitationID: "should-not-send"}, nil
	}
	resolve := func(_ context.Context, _, _ string) (string, error) {
		return "", unipile.ErrRateLimited // transient
	}

	processLinkedInQueue(ctx, repository.New(pool), send, resolve, pacer.Standard())

	// No invite went out.
	if sendCalls != 0 {
		t.Errorf("send invoked despite a transient resolve failure: calls=%d", sendCalls)
	}
	// Lead untouched: still queued at step 0.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead status: %v", err)
	}
	if status != "queued" {
		t.Errorf("status=%q want queued (transient resolve leaves it in place)", status)
	}
	// No event written on a transient failure.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID); n != 0 {
		t.Errorf("linkedin_events=%d want 0 (transient resolve writes none)", n)
	}
	// Pacing allowance not consumed — the account's invite counters stay at 0.
	var weekly, daily int32
	if err := pool.QueryRow(ctx, `SELECT weekly_invite_count, daily_invite_count FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&weekly, &daily); err != nil {
		t.Fatalf("scan counters: %v", err)
	}
	if weekly != 0 || daily != 0 {
		t.Errorf("invite counters=(weekly %d, daily %d) want (0, 0) — resolve failed before the gate", weekly, daily)
	}
}

// TestProcessLinkedInQueue_PermanentResolveFailsLead wires issue #5's permanent-
// resolve path into the invite loop (issue #6): an unresolvable profile
// (ErrNotFound) moves the campaign_lead to a terminal 'failed' status and writes
// a single 'failed' linkedin_event, without ever sending an invite or consuming
// pacing. The lead drops out of the due queue and is not retried.
func TestProcessLinkedInQueue_PermanentResolveFailsLead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)

	sendCalls := 0
	send := func(_ context.Context, _ unipile.InviteParams) (*unipile.InviteResult, error) {
		sendCalls++
		return &unipile.InviteResult{InvitationID: "should-not-send"}, nil
	}
	resolve := func(_ context.Context, _, _ string) (string, error) {
		return "", fmt.Errorf("%w: profile gone", unipile.ErrNotFound) // permanent
	}

	processLinkedInQueue(ctx, repository.New(pool), send, resolve, pacer.Standard())

	// No invite went out.
	if sendCalls != 0 {
		t.Errorf("send invoked despite a permanent resolve failure: calls=%d", sendCalls)
	}
	// Lead moved to terminal 'failed'.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead status: %v", err)
	}
	if status != "failed" {
		t.Errorf("status=%q want failed (permanent resolve fails the lead terminally)", status)
	}
	// Exactly one 'failed' event, no invite_sent.
	var failed, sent int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE event_type='failed'),
		       COUNT(*) FILTER (WHERE event_type='invite_sent')
		FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID).Scan(&failed, &sent); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if failed != 1 {
		t.Errorf("failed events=%d want 1", failed)
	}
	if sent != 0 {
		t.Errorf("invite_sent events=%d want 0", sent)
	}
	// Pacing not consumed.
	var weekly int32
	if err := pool.QueryRow(ctx, `SELECT weekly_invite_count FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&weekly); err != nil {
		t.Fatalf("scan counter: %v", err)
	}
	if weekly != 0 {
		t.Errorf("weekly_invite_count=%d want 0 (nothing sent)", weekly)
	}
}
