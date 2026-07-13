//go:build integration

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// reconcileFixture is the graph the reconcile poll backstops: one connected
// account with two leads — one parked awaiting acceptance (matched by the
// prospect's cached member id showing up in the account's connections) and one
// active mid-sequence with a chat bound (matched by its chat id). The poll
// detects the acceptance and the missed reply and applies both. The accept
// lead keeps its invitation id bound (the withdrawal sweep still keys on it)
// even though acceptance is now detected by member id, not invitation id.
type reconcileFixture struct {
	accountUnipileID string
	acceptLeadID     uuid.UUID
	acceptInvID      string
	acceptMemberID   string
	replyLeadID      uuid.UUID
	replyChatID      string
	replyPersonID    uuid.UUID
	tenantID         uuid.UUID
}

func newLinkedInReconcileFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) reconcileFixture {
	t.Helper()
	run := uuid.NewString()
	f := reconcileFixture{
		accountUnipileID: "acc_unipile_" + run,
		acceptLeadID:     uuid.New(),
		acceptInvID:      "inv_" + run,
		acceptMemberID:   "ACoAA_" + run,
		replyLeadID:      uuid.New(),
		replyChatID:      "chat_" + run,
		replyPersonID:    uuid.New(),
		tenantID:         uuid.New(),
	}
	userID := uuid.New()
	acceptPersonID := uuid.New()
	acceptIdentID := uuid.New()
	acceptMemberIdentID := uuid.New()
	replyIdentID := uuid.New()
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
		f.tenantID, "Reconcile Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	// Two persons: one for the accept lead, one for the reply lead.
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		acceptPersonID, "Ada Lovelace", "ada lovelace", "Ada", "Lovelace")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		acceptIdentID, acceptPersonID, "https://www.linkedin.com/in/ada-"+run)
	// Cached member id (resolved at invite time, issue #5) — the key the
	// connections check matches acceptance on.
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_member_id', $3, FALSE)`,
		acceptMemberIdentID, acceptPersonID, f.acceptMemberID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		f.replyPersonID, "Grace Hopper", "grace hopper", "Grace", "Hopper")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		replyIdentID, f.replyPersonID, "https://www.linkedin.com/in/grace-"+run)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "Reconcile Test Play")
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel) VALUES ($1, $2, $3, $4, 'active', 'linkedin')`,
		campaignID, f.tenantID, playID, "Reconcile Test Campaign")
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		accountID, f.tenantID, f.accountUnipileID)
	// Accept lead: parked awaiting acceptance, bound to the invitation.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, linkedin_invitation_id)
		VALUES ($1, $2, $3, 'awaiting_accept', 0, $4, $5)`,
		f.acceptLeadID, campaignID, acceptPersonID, accountID, f.acceptInvID)
	// Reply lead: active mid-sequence with a chat bound.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, linkedin_chat_id, accepted_at)
		VALUES ($1, $2, $3, 'active', 1, $4, $5, NOW())`,
		f.replyLeadID, campaignID, f.replyPersonID, accountID, f.replyChatID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id IN ($1, $2)`, f.acceptLeadID, f.replyLeadID)
		_, _ = pool.Exec(c, `DELETE FROM unsubscribes WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id IN ($1, $2)`, f.acceptLeadID, f.replyLeadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE id IN ($1, $2, $3)`, acceptIdentID, acceptMemberIdentID, replyIdentID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id IN ($1, $2)`, acceptPersonID, f.replyPersonID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

// TestReconcileLinkedInOnce_AppliesAcceptAndReply is issue #7's reconcile
// tracer: a poll tick lists the account's connections, matches the prospect's
// cached member id to the parked awaiting_accept lead, and flips it active/
// step-1 with an 'accepted' event (issue #7); it also detects one missed reply
// (from the Unipile activity stub) and flips that lead 'replied' + suppresses
// the person (issue #6). Running the tick again is idempotent: no second
// 'accepted' or 'replied' event, no second suppression row — the same
// awaiting_accept / status<>'replied' gates the webhook path uses.
func TestReconcileLinkedInOnce_AppliesAcceptAndReply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInReconcileFixture(t, ctx, pool)

	// The connections stub reports the account's current connections — the
	// accepted prospect's member id is present; nothing for any other account.
	connections := func(_ context.Context, unipileAccountID string) ([]string, error) {
		if unipileAccountID != f.accountUnipileID {
			return nil, nil
		}
		return []string{"ACoAA_someone_else", f.acceptMemberID}, nil
	}
	// The activity stub reports the account's missed reply, and nothing for
	// any other account that happens to be listed.
	activity := func(_ context.Context, unipileAccountID string) (unipile.AccountActivity, error) {
		if unipileAccountID != f.accountUnipileID {
			return unipile.AccountActivity{}, nil
		}
		return unipile.AccountActivity{
			Replies: []unipile.InboundReply{{ChatID: f.replyChatID, MessageID: "msg_reconcile_1"}},
		}, nil
	}

	q := repository.New(pool)
	supp := suppression.New(pool)

	// Two ticks — the second must change nothing (idempotent with the webhook).
	reconcileLinkedInOnce(ctx, q, supp, connections, activity)
	reconcileLinkedInOnce(ctx, q, supp, connections, activity)

	// Accept lead flipped active/step-1, accepted_at stamped, first DM
	// scheduled for now (so the DM tick picks it up), with exactly one
	// 'accepted' event.
	var acceptStatus string
	var acceptStep int32
	var acceptedStamped, dmScheduled bool
	if err := pool.QueryRow(ctx, `SELECT status, current_step, accepted_at IS NOT NULL, (next_send_at IS NOT NULL AND next_send_at <= NOW()) FROM campaign_leads WHERE id = $1`, f.acceptLeadID).Scan(&acceptStatus, &acceptStep, &acceptedStamped, &dmScheduled); err != nil {
		t.Fatalf("scan accept lead: %v", err)
	}
	if acceptStatus != "active" || acceptStep != 1 {
		t.Errorf("accept lead status=%q step=%d want active/1", acceptStatus, acceptStep)
	}
	if !acceptedStamped {
		t.Error("accept lead accepted_at not stamped")
	}
	if !dmScheduled {
		t.Error("accept lead next_send_at not scheduled for immediate first DM")
	}
	var acceptedEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'accepted'`, f.acceptLeadID).Scan(&acceptedEvents); err != nil {
		t.Fatalf("count accepted: %v", err)
	}
	if acceptedEvents != 1 {
		t.Errorf("accepted events=%d want 1 (idempotent across two ticks)", acceptedEvents)
	}

	// Reply lead flipped 'replied' with exactly one 'replied' event.
	var replyStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.replyLeadID).Scan(&replyStatus); err != nil {
		t.Fatalf("scan reply lead: %v", err)
	}
	if replyStatus != "replied" {
		t.Errorf("reply lead status=%q want replied", replyStatus)
	}
	var repliedEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'replied'`, f.replyLeadID).Scan(&repliedEvents); err != nil {
		t.Fatalf("count replied: %v", err)
	}
	if repliedEvents != 1 {
		t.Errorf("replied events=%d want 1 (idempotent across two ticks)", repliedEvents)
	}

	// Person suppressed exactly once.
	var unsubCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM unsubscribes WHERE tenant_id = $1 AND person_id = $2`, f.tenantID, f.replyPersonID).Scan(&unsubCount); err != nil {
		t.Fatalf("count unsub: %v", err)
	}
	if unsubCount != 1 {
		t.Errorf("unsubscribes rows=%d want 1 (idempotent)", unsubCount)
	}
}
