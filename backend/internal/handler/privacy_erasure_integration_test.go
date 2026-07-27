//go:build integration

package handler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/ingest"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// TestRunErasure_UnifiedLinkedInRows proves issue #07's AC4: erasure keyed on
// a LinkedIn URL finds the unified 'linkedin_url' person rows (not the legacy
// 'linkedin' type), and the blocklist hash it writes is the exact hash the
// ingest resolver checks on re-ingest — so an erased LinkedIn prospect cannot
// be re-ingested. runErasure receives the already-canonicalized value, which
// RequestErasure now produces via liurl.Canonical.
func TestRunErasure_UnifiedLinkedInRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool := acceptTestPool(t, ctx)

	run := uuid.NewString()
	personID := uuid.New()
	// A unified row: the canonical full-URL form under identifier_type
	// 'linkedin_url', exactly what the resolver / search / seed now write.
	canonicalURL := "https://www.linkedin.com/in/erasure-" + run

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}
	mustExec(`INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, 'Erasure', 'Target')`,
		personID, "Erasure Target "+run, "erasure target "+run)
	mustExec(`INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) VALUES ($1, 'linkedin_url', $2, TRUE)`,
		personID, canonicalURL)

	blockHash := ingest.HashIdentifier(canonicalURL)
	t.Cleanup(func() {
		c := context.Background()
		_, _ = pool.Exec(c, `DELETE FROM deletion_blocklist WHERE identifier_hash = $1`, blockHash)
		_, _ = pool.Exec(c, `DELETE FROM person_identifiers WHERE person_id = $1`, personID)
		_, _ = pool.Exec(c, `DELETE FROM persons WHERE id = $1`, personID)
	})

	h := NewPrivacyHandler(repository.New(pool), pool, nil, "")

	personIDs, blockedTypes, err := h.runErasure(ctx, "", canonicalURL, "", "", false, "ac4-test")
	if err != nil {
		t.Fatalf("runErasure: %v", err)
	}

	// Found the unified row — empty if the query still filtered 'linkedin'.
	found := false
	for _, id := range personIDs {
		if id.Bytes == personID {
			found = true
		}
	}
	if !found {
		t.Errorf("runErasure did not find the unified linkedin_url person (personIDs=%v)", personIDs)
	}
	if len(blockedTypes) != 1 || blockedTypes[0] != "linkedin" {
		t.Errorf("blockedTypes=%v want [linkedin]", blockedTypes)
	}

	// The person was hard-deleted.
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM persons WHERE id = $1`, personID).Scan(&remaining); err != nil {
		t.Fatalf("count persons: %v", err)
	}
	if remaining != 0 {
		t.Errorf("person not deleted: count=%d", remaining)
	}

	// The blocklist carries the SAME hash the resolver computes for a
	// re-ingest of this URL (HashIdentifier of the canonical value), so
	// ingest's AnyHashBlocked gate blocks it.
	blocked, err := repository.New(pool).AnyHashBlocked(ctx, []string{blockHash})
	if err != nil {
		t.Fatalf("AnyHashBlocked: %v", err)
	}
	if !blocked {
		t.Error("erased LinkedIn identifier is not on the blocklist — re-ingest would not be blocked")
	}
}
