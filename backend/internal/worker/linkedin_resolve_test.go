//go:build integration

package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// personIDForURL returns the person that owns the given linkedin_url
// identifier, so the resolver tests can key cache assertions on it.
func personIDForURL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, url string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT person_id FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1`,
		url).Scan(&id); err != nil {
		t.Fatalf("look up person_id for %q: %v", url, err)
	}
	return id
}

// TestResolveMemberID_ReadThrough is the tracer bullet for issue #5: a cache
// miss calls Unipile exactly once and persists the member id as a
// linkedin_member_id person_identifier; a second call for the same person is
// served from the cache without touching Unipile.
func TestResolveMemberID_ReadThrough(t *testing.T) {
	ctx := context.Background()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)
	personID := personIDForURL(t, ctx, pool, f.profileURL)

	const wantMemberID = "ACoAABtracer0001"
	calls := 0
	resolve := func(_ context.Context, profileURL, accountID string) (string, error) {
		calls++
		if profileURL != f.profileURL {
			t.Fatalf("resolve got profileURL %q, want %q", profileURL, f.profileURL)
		}
		if accountID != f.unipileID {
			t.Fatalf("resolve got accountID %q, want %q", accountID, f.unipileID)
		}
		return wantMemberID, nil
	}

	q := repository.New(pool)
	in := resolveInput{
		CampaignLeadID: pgtype.UUID{Bytes: f.leadID, Valid: true},
		PersonID:       pgtype.UUID{Bytes: personID, Valid: true},
		LinkedinURL:    f.profileURL,
		AccountID:      f.unipileID,
		CurrentStep:    0,
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM person_identifiers WHERE identifier_type='linkedin_member_id' AND person_id=$1`, personID)
	})

	// Miss: calls Unipile once, returns the member id.
	got, ok := resolveMemberID(ctx, q, resolve, in)
	if !ok || got != wantMemberID {
		t.Fatalf("first resolve = (%q, %v), want (%q, true)", got, ok, wantMemberID)
	}
	if calls != 1 {
		t.Fatalf("after miss, Unipile calls = %d, want 1", calls)
	}

	// Persisted as a linkedin_member_id identifier on that person.
	var persisted int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM person_identifiers WHERE person_id=$1 AND identifier_type='linkedin_member_id' AND identifier_value=$2`,
		personID, wantMemberID).Scan(&persisted); err != nil {
		t.Fatalf("query persisted identifier: %v", err)
	}
	if persisted != 1 {
		t.Fatalf("linkedin_member_id rows = %d, want 1", persisted)
	}

	// Hit: served from cache, no further Unipile call.
	got2, ok2 := resolveMemberID(ctx, q, resolve, in)
	if !ok2 || got2 != wantMemberID {
		t.Fatalf("second resolve = (%q, %v), want (%q, true)", got2, ok2, wantMemberID)
	}
	if calls != 1 {
		t.Fatalf("after cache hit, Unipile calls = %d, want 1", calls)
	}
}

// TestResolveMemberID_TransientLeavesLeadQueued covers issue #5's transient
// path: an upstream blip (rate limit / 5xx / timeout) must leave the lead
// exactly as it was — still queued, no member id cached, no failure event —
// and must not consume the account's invite pacing. The next tick retries.
func TestResolveMemberID_TransientLeavesLeadQueued(t *testing.T) {
	ctx := context.Background()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)
	personID := personIDForURL(t, ctx, pool, f.profileURL)

	resolve := func(_ context.Context, _, _ string) (string, error) {
		return "", unipile.ErrRateLimited // transient
	}
	q := repository.New(pool)
	got, ok := resolveMemberID(ctx, q, resolve, resolveInput{
		CampaignLeadID: pgtype.UUID{Bytes: f.leadID, Valid: true},
		PersonID:       pgtype.UUID{Bytes: personID, Valid: true},
		LinkedinURL:    f.profileURL,
		AccountID:      f.unipileID,
		CurrentStep:    0,
	})
	if ok || got != "" {
		t.Fatalf(`transient resolve = (%q, %v), want ("", false)`, got, ok)
	}

	// Lead untouched: still queued.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("read lead status: %v", err)
	}
	if status != "queued" {
		t.Fatalf("lead status = %q, want queued (unchanged)", status)
	}

	// No failure event written.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID); n != 0 {
		t.Fatalf("linkedin_events rows = %d, want 0 (no event on transient)", n)
	}

	// Nothing cached — the next tick will resolve again.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM person_identifiers WHERE person_id=$1 AND identifier_type='linkedin_member_id'`, personID); n != 0 {
		t.Fatalf("linkedin_member_id rows = %d, want 0", n)
	}

	// Pacing not consumed: the account's invite counters stay at 0.
	var weekly, daily int
	if err := pool.QueryRow(ctx,
		`SELECT weekly_invite_count, daily_invite_count FROM linkedin_accounts WHERE id=$1`,
		f.accountID).Scan(&weekly, &daily); err != nil {
		t.Fatalf("read counters: %v", err)
	}
	if weekly != 0 || daily != 0 {
		t.Fatalf("invite counters = (weekly %d, daily %d), want (0, 0)", weekly, daily)
	}
}

// TestResolveMemberID_PermanentFailsLeadTerminally covers issue #5's
// permanent path: an unresolvable profile (ErrNotFound) writes a 'failed'
// linkedin_event attributed to the lead's step and moves the campaign_lead to
// a terminal 'failed' status — a new free-text value in the status column (no
// migration) that drops the lead out of every due query so it is not retried.
func TestResolveMemberID_PermanentFailsLeadTerminally(t *testing.T) {
	ctx := context.Background()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)
	personID := personIDForURL(t, ctx, pool, f.profileURL)

	resolve := func(_ context.Context, _, _ string) (string, error) {
		return "", fmt.Errorf("%w: profile gone", unipile.ErrNotFound) // permanent
	}
	q := repository.New(pool)
	got, ok := resolveMemberID(ctx, q, resolve, resolveInput{
		CampaignLeadID: pgtype.UUID{Bytes: f.leadID, Valid: true},
		PersonID:       pgtype.UUID{Bytes: personID, Valid: true},
		LinkedinURL:    f.profileURL,
		AccountID:      f.unipileID,
		CurrentStep:    0,
	})
	if ok || got != "" {
		t.Fatalf(`permanent resolve = (%q, %v), want ("", false)`, got, ok)
	}

	// Lead moved to terminal 'failed'.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM campaign_leads WHERE id=$1`, f.leadID).Scan(&status); err != nil {
		t.Fatalf("read lead status: %v", err)
	}
	if status != "failed" {
		t.Fatalf("lead status = %q, want failed (terminal)", status)
	}

	// A single 'failed' linkedin_event recorded, attributed to the lead's step.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM linkedin_events WHERE campaign_lead_id=$1`, f.leadID); n != 1 {
		t.Fatalf("linkedin_events rows = %d, want 1", n)
	}
	var eventType string
	var step int32
	if err := pool.QueryRow(ctx,
		`SELECT event_type, step FROM linkedin_events WHERE campaign_lead_id=$1`,
		f.leadID).Scan(&eventType, &step); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if eventType != "failed" {
		t.Fatalf("event_type = %q, want failed", eventType)
	}
	if step != 0 {
		t.Fatalf("event step = %d, want 0 (lead's current_step)", step)
	}

	// Nothing cached — there was no member id to remember.
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM person_identifiers WHERE person_id=$1 AND identifier_type='linkedin_member_id'`, personID); n != 0 {
		t.Fatalf("linkedin_member_id rows = %d, want 0", n)
	}
}

// TestResolveMemberID_PersistOnceAcrossCampaigns proves the cache is keyed on
// the person, not the lead: once resolved, a second campaign targeting the
// same person (a different campaign_lead) is served from the cache — no second
// Unipile lookup and no duplicate identifier row. This is issue #5's
// "persist-once, idempotent — the same person is never looked up twice, even
// across campaigns."
func TestResolveMemberID_PersistOnceAcrossCampaigns(t *testing.T) {
	ctx := context.Background()
	pool := linkedInTestPool(t, ctx)
	f := newLinkedInInviteFixture(t, ctx, pool)
	personID := personIDForURL(t, ctx, pool, f.profileURL)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM person_identifiers WHERE identifier_type='linkedin_member_id' AND person_id=$1`, personID)
	})

	const memberID = "ACoAABpersistonce7"
	calls := 0
	resolve := func(_ context.Context, _, _ string) (string, error) {
		calls++
		return memberID, nil
	}
	q := repository.New(pool)
	base := resolveInput{
		CampaignLeadID: pgtype.UUID{Bytes: f.leadID, Valid: true},
		PersonID:       pgtype.UUID{Bytes: personID, Valid: true},
		LinkedinURL:    f.profileURL,
		AccountID:      f.unipileID,
		CurrentStep:    0,
	}

	// First campaign: miss → resolve + persist.
	if got, ok := resolveMemberID(ctx, q, resolve, base); !ok || got != memberID {
		t.Fatalf("first resolve = (%q, %v), want (%q, true)", got, ok, memberID)
	}

	// Second campaign, same person, different lead id: cache hit.
	second := base
	second.CampaignLeadID = pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if got, ok := resolveMemberID(ctx, q, resolve, second); !ok || got != memberID {
		t.Fatalf("second-campaign resolve = (%q, %v), want (%q, true)", got, ok, memberID)
	}
	if calls != 1 {
		t.Fatalf("Unipile calls = %d, want 1 (person resolved once across campaigns)", calls)
	}
	if n := countRows(t, ctx, pool, `SELECT count(*) FROM person_identifiers WHERE person_id=$1 AND identifier_type='linkedin_member_id'`, personID); n != 1 {
		t.Fatalf("linkedin_member_id rows = %d, want 1 (persist-once)", n)
	}
}

// countRows runs a single-count query and returns the scalar.
func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", q, err)
	}
	return n
}
