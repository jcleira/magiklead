//go:build integration

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// acceptFixture is the post-invite graph issue #5's acceptance webhook
// acts on: a LinkedIn campaign with a campaign_lead parked in
// 'awaiting_accept' at step 0, bound to a connected account and a Unipile
// invitation id. The acceptance webhook matches on that invitation id.
//
// Run with:
//
//	devpods exec api go test -tags=integration ./internal/handler/...
type acceptFixture struct {
	tenantID     uuid.UUID
	leadID       uuid.UUID
	accountID    uuid.UUID
	invitationID string
}

func newAcceptFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) acceptFixture {
	t.Helper()
	run := uuid.NewString()
	f := acceptFixture{
		tenantID:     uuid.New(),
		leadID:       uuid.New(),
		accountID:    uuid.New(),
		invitationID: "inv_" + run,
	}
	userID := uuid.New()
	personID := uuid.New()
	identID := uuid.New()
	playID := uuid.New()
	campaignID := uuid.New()
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
		f.tenantID, "Accept Test "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		userID, f.tenantID)
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, "Grace Hopper", "grace hopper", "Grace", "Hopper")
	mustExec(`INSERT INTO person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1, $2, 'linkedin_url', $3, TRUE)`,
		identID, personID, "https://www.linkedin.com/in/grace-"+run)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`,
		playID, f.tenantID, "Accept Test Play")
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel) VALUES ($1, $2, $3, $4, 'active', 'linkedin')`,
		campaignID, f.tenantID, playID, "Accept Test Campaign")
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
		f.accountID, f.tenantID, "acc_unipile_"+run)
	// Parked awaiting acceptance, bound to the account + invitation — the
	// exact state MarkLinkedInInviteSent (issue #4) leaves behind.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, linkedin_invitation_id)
		VALUES ($1, $2, $3, 'awaiting_accept', 0, $4, $5)`,
		f.leadID, campaignID, personID, f.accountID, f.invitationID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM linkedin_events WHERE campaign_lead_id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id = $1`, f.leadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id = $1`, campaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, f.accountID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE id = $1`, identID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
	})
	return f
}

func acceptTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// signedAcceptWebhook builds an authenticated invitation-accepted request
// the real module parses end to end. Auth is the static Unipile-Auth
// header (== the webhook secret).
func signedAcceptWebhook(t *testing.T, _ *unipile.Module, invitationID, accountID string) *http.Request {
	t.Helper()
	body := []byte(`{"invitation_id":"` + invitationID + `","account_id":"` + accountID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	return req
}

// TestUnipile_Webhook_InvitationAccepted_FlipsLeadAndWritesEvent is the
// acceptance tracer through every layer: a signed invitation-accepted
// webhook is verified, parsed, matched to the parked lead by its Unipile
// invitation id, and the lead transitions awaiting_accept → active with
// accepted_at + next_send_at=NOW() + current_step=1, plus an 'accepted'
// linkedin_event (the acceptance-rate signal issue #7 counts). Asserts via
// DB.
func TestUnipile_Webhook_InvitationAccepted_FlipsLeadAndWritesEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newAcceptFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedAcceptWebhook(t, mod, f.invitationID, "acc_unipile"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}

	// Lead flipped to active, scheduled now, advanced to step 1, accepted_at set.
	var status string
	var step int32
	var nextSendAt, acceptedAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT status, current_step, next_send_at, accepted_at FROM campaign_leads WHERE id = $1`, f.leadID).
		Scan(&status, &step, &nextSendAt, &acceptedAt); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "active" {
		t.Errorf("status=%q want active", status)
	}
	if step != 1 {
		t.Errorf("current_step=%d want 1", step)
	}
	if !nextSendAt.Valid {
		t.Errorf("next_send_at is NULL want NOW() so the DM tick picks it up")
	}
	if !acceptedAt.Valid {
		t.Errorf("accepted_at is NULL want set")
	}

	// 'accepted' event written exactly once — the signal #7's acceptance
	// rate is computed from.
	var acceptedEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'accepted'`, f.leadID).Scan(&acceptedEvents); err != nil {
		t.Fatalf("count accepted events: %v", err)
	}
	if acceptedEvents != 1 {
		t.Errorf("accepted events=%d want 1", acceptedEvents)
	}
}

// TestUnipile_Webhook_InvitationAccepted_Idempotent proves replay safety:
// delivering the same acceptance twice flips the lead once and writes one
// 'accepted' event. The second delivery matches no awaiting_accept row (the
// WHERE gate in MarkLinkedInAccepted), so nothing is double-counted.
func TestUnipile_Webhook_InvitationAccepted_Idempotent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newAcceptFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.Webhook(rr, signedAcceptWebhook(t, mod, f.invitationID, "acc_unipile"))
		if rr.Code != http.StatusOK {
			t.Fatalf("delivery %d status=%d want 200", i, rr.Code)
		}
	}

	var acceptedEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM linkedin_events WHERE campaign_lead_id = $1 AND event_type = 'accepted'`, f.leadID).Scan(&acceptedEvents); err != nil {
		t.Fatalf("count accepted events: %v", err)
	}
	if acceptedEvents != 1 {
		t.Errorf("accepted events=%d want 1 (idempotent on replay)", acceptedEvents)
	}
}

// TestUnipile_Webhook_InvitationAccepted_UnknownID_NoOp covers the safe
// no-op: an acceptance for an invitation id we never sent matches no lead,
// writes nothing, and still returns 200 (so Unipile doesn't retry forever).
func TestUnipile_Webhook_InvitationAccepted_UnknownID_NoOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)
	f := newAcceptFixture(t, ctx, pool)

	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	h := &UnipileHandler{svc: mod, queries: repository.New(pool), stateSecret: secret}

	rr := httptest.NewRecorder()
	h.Webhook(rr, signedAcceptWebhook(t, mod, "inv_never_sent_"+uuid.NewString(), "acc_unipile"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (safe no-op)", rr.Code)
	}

	// The real fixture lead is untouched — still parked.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id = $1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("scan lead: %v", err)
	}
	if status != "awaiting_accept" {
		t.Errorf("status=%q want awaiting_accept (unknown id must not touch other leads)", status)
	}
}
