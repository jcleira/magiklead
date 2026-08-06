//go:build integration

package ingest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// ingestTestPool mirrors the handler/worker integration pool helpers: it
// skips when DATABASE_URL is unset and closes the pool on cleanup.
func ingestTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
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

// TestCrossSourceVisibility is issue #07's tracer. A person planted through
// the ingest resolver must be visible to every LinkedIn-rail reader —
// ListLinkedInURLsForPersons, GetDueLinkedInInviteLeads, GetDueLinkedInDMLeads.
// Those readers filter identifier_type = 'linkedin_url'; before unification the
// resolver wrote 'linkedin' + a scheme-less value, so an ingest-created person
// was invisible to LinkedIn search and never selected for sends. This proves
// the writer now converges on 'linkedin_url' + the canonical full URL, end to
// end through the real resolver and the real generated readers.
func TestCrossSourceVisibility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := ingestTestPool(t, ctx)
	q := repository.New(pool)

	run := uuid.NewString()
	name := "Tracer " + run
	// Deliberately messy input: uppercase scheme/host/slug, trailing slash,
	// query, fragment. The resolver must canonicalize it end to end.
	rawURL := "HTTP://WWW.LinkedIn.COM/in/Tracer-" + run + "/?utm_source=x#frag"
	wantURL := "https://www.linkedin.com/in/tracer-" + run

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}

	// Ingest FK chain so Resolve's evidence write (NOT NULL FK to
	// source_records) has a real row to reference.
	sourceID := uuid.New()
	rawIngestID := uuid.New()
	sourceRecordID := uuid.New()
	mustExec(`INSERT INTO sources (id, name, type) VALUES ($1, $2, 'test')`, sourceID, "tracer-"+run)
	mustExec(`INSERT INTO raw_ingests (id, source_id, file_url, checksum, size_bytes) VALUES ($1, $2, 's3://tracer', $3, 0)`, rawIngestID, sourceID, "chk-"+run)
	mustExec(`INSERT INTO source_records (id, source_id, raw_ingest_id, fields) VALUES ($1, $2, $3, '{}'::jsonb)`, sourceRecordID, sourceID, rawIngestID)

	// Plant the person THROUGH the resolver.
	if err := NewDefaultResolver().Resolve(ctx, q, ResolveInput{
		SourceID:       pgUUID(sourceID),
		SourceName:     "tracer",
		SourceRecordID: pgUUID(sourceRecordID),
		Record: SourceRecord{Fields: map[string]any{
			"name":         name,
			"linkedin_url": rawURL,
		}},
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var personID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM persons WHERE canonical_name = $1`, name).Scan(&personID); err != nil {
		t.Fatalf("find planted person: %v", err)
	}

	// Campaign graph so the due-lead readers have rows to select. The invite
	// lead and DM lead are the same person in mutually exclusive states, so
	// they need separate campaigns (uniq_campaign_leads_campaign_person).
	userID := uuid.New()
	tenantID := uuid.New()
	playID := uuid.New()
	inviteCampaignID := uuid.New()
	dmCampaignID := uuid.New()
	accountID := uuid.New()
	inviteLeadID := uuid.New()
	dmLeadID := uuid.New()

	mustExec(`INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`, userID, "user_tracer_"+run, "tracer-"+run+"@example.com")
	mustExec(`INSERT INTO tenants (id, name) VALUES ($1, $2)`, tenantID, "Tracer "+run)
	mustExec(`INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`, userID, tenantID)
	mustExec(`INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY['linkedin']::text[])`, playID, tenantID, "Tracer Play")
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel) VALUES ($1, $2, $3, 'Invite Camp', 'active', 'linkedin')`, inviteCampaignID, tenantID, playID)
	mustExec(`INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel) VALUES ($1, $2, $3, 'DM Camp', 'active', 'linkedin')`, dmCampaignID, tenantID, playID)
	mustExec(`INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`, accountID, tenantID, "acc_tracer_"+run)
	// Invite lead: due now (step 0, queued, no next_send_at).
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step) VALUES ($1, $2, $3, 'queued', 0)`, inviteLeadID, inviteCampaignID, personID)
	// DM lead: accepted, mid-sequence, first DM overdue, bound to an active account.
	mustExec(`INSERT INTO campaign_leads (id, campaign_id, person_id, status, current_step, linkedin_account_id, next_send_at) VALUES ($1, $2, $3, 'active', 1, $4, NOW() - INTERVAL '1 minute')`, dmLeadID, dmCampaignID, personID, accountID)

	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM campaign_leads WHERE id IN ($1, $2)`, inviteLeadID, dmLeadID)
		_, _ = pool.Exec(c, `DELETE FROM campaigns WHERE id IN ($1, $2)`, inviteCampaignID, dmCampaignID)
		_, _ = pool.Exec(c, `DELETE FROM plays WHERE id = $1`, playID)
		_, _ = pool.Exec(c, `DELETE FROM linkedin_accounts WHERE id = $1`, accountID)
		_, _ = pool.Exec(c, `DELETE FROM user_tenants WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(c, `DELETE FROM tenants WHERE id = $1`, tenantID)
		_, _ = pool.Exec(c, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE person_id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
		// evidence cascades from source_records (ON DELETE CASCADE).
		_, _ = pool.Exec(c, `DELETE FROM source_records WHERE id = $1`, sourceRecordID)
		_, _ = pool.Exec(c, `DELETE FROM raw_ingests WHERE id = $1`, rawIngestID)
		_, _ = pool.Exec(c, `DELETE FROM sources WHERE id = $1`, sourceID)
	})

	// AC6.1 — the profile-link reader returns the canonical URL.
	urls, err := q.ListLinkedInURLsForPersons(ctx, []pgtype.UUID{personID})
	if err != nil {
		t.Fatalf("ListLinkedInURLsForPersons: %v", err)
	}
	if got := listURLForPerson(urls, personID); got != wantURL {
		t.Errorf("ListLinkedInURLsForPersons linkedin_url = %q, want %q", got, wantURL)
	}

	// AC6.2 — the invite due-query selects the person with the canonical URL.
	invites, err := q.GetDueLinkedInInviteLeads(ctx, 500)
	if err != nil {
		t.Fatalf("GetDueLinkedInInviteLeads: %v", err)
	}
	if got, ok := inviteURLForPerson(invites, personID); !ok || got != wantURL {
		t.Errorf("GetDueLinkedInInviteLeads: selected=%v url=%q, want selected with %q", ok, got, wantURL)
	}

	// AC6.3 — the DM due-query selects the person with the canonical URL.
	dms, err := q.GetDueLinkedInDMLeads(ctx, 500)
	if err != nil {
		t.Fatalf("GetDueLinkedInDMLeads: %v", err)
	}
	if got, ok := dmURLForPerson(dms, personID); !ok || got != wantURL {
		t.Errorf("GetDueLinkedInDMLeads: selected=%v url=%q, want selected with %q", ok, got, wantURL)
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func samePerson(a, b pgtype.UUID) bool { return a.Valid && b.Valid && a.Bytes == b.Bytes }

func listURLForPerson(rows []repository.ListLinkedInURLsForPersonsRow, id pgtype.UUID) string {
	for _, r := range rows {
		if samePerson(r.PersonID, id) {
			return r.LinkedinUrl
		}
	}
	return ""
}

func inviteURLForPerson(rows []repository.GetDueLinkedInInviteLeadsRow, id pgtype.UUID) (string, bool) {
	for _, r := range rows {
		if samePerson(r.PersonID, id) {
			return r.LinkedinUrl, true
		}
	}
	return "", false
}

func dmURLForPerson(rows []repository.GetDueLinkedInDMLeadsRow, id pgtype.UUID) (string, bool) {
	for _, r := range rows {
		if samePerson(r.PersonID, id) {
			return r.LinkedinUrl, true
		}
	}
	return "", false
}
