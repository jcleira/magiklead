//go:build integration

package worker

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

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
	defer pool.Close()

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

	processQueue(ctx, repository.New(pool), suppression.New(pool))

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
