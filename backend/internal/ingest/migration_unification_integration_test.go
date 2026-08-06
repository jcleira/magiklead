//go:build integration

package ingest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestMigration030_LinkedInUnification runs the actual 030 up migration over
// a copy of person_identifiers seeded with legacy-shaped rows, in an isolated
// schema so it never touches real data. It proves issue #07's AC5: legacy
// 'linkedin' rows are rewritten to 'linkedin_url' + the canonical value, and a
// legacy row whose canonical target collides with an existing 'linkedin_url'
// row (identifier_value is globally UNIQUE) is dropped with the survivor left
// intact.
func TestMigration030_LinkedInUnification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := ingestTestPool(t, ctx)

	// Read the real migration file relative to this source file, so the test
	// exercises the shipped artifact rather than a copy.
	_, thisFile, _, _ := runtime.Caller(0)
	upPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations", "030_linkedin_identifier_unification.up.sql")
	upSQL, err := os.ReadFile(upPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	const schema = "migtest_030"
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}

	mustExec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	mustExec(`CREATE SCHEMA ` + schema)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) })
	// Copy the real table's shape — INCLUDING ALL brings the UNIQUE index on
	// identifier_value that drives the collision branch. LIKE never copies the
	// persons FK, so arbitrary person_id UUIDs are fine.
	mustExec(`CREATE TABLE ` + schema + `.person_identifiers (LIKE public.person_identifiers INCLUDING ALL)`)

	idA, idB, idC, idD := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ins := func(id uuid.UUID, typ, val string) {
		mustExec(`INSERT INTO `+schema+`.person_identifiers (id, person_id, identifier_type, identifier_value, is_primary) VALUES ($1,$2,$3,$4,FALSE)`,
			id, uuid.New(), typ, val)
	}
	// A — legacy 'linkedin', no collision → rewritten.
	ins(idA, "linkedin", "linkedin.com/in/alpha")
	// B — existing canonical 'linkedin_url'.
	ins(idB, "linkedin_url", "https://www.linkedin.com/in/bravo")
	// C — legacy 'linkedin' whose canonical target == B's value → dropped.
	ins(idC, "linkedin", "linkedin.com/in/bravo")
	// D — control 'linkedin_url', untouched.
	ins(idD, "linkedin_url", "https://www.linkedin.com/in/delta")

	// Run the real migration with the isolated schema on the search_path so
	// its unqualified person_identifiers references hit our copy. SET LOCAL is
	// tx-scoped, so the pooled connection resets cleanly on commit.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+schema+`, public`); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	if _, err := tx.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("run migration: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// A rewritten to unified type + canonical value.
	var typ, val string
	if err := pool.QueryRow(ctx, `SELECT identifier_type, identifier_value FROM `+schema+`.person_identifiers WHERE id=$1`, idA).Scan(&typ, &val); err != nil {
		t.Fatalf("scan A: %v", err)
	}
	if typ != "linkedin_url" || val != "https://www.linkedin.com/in/alpha" {
		t.Errorf("row A = (%q, %q), want (linkedin_url, https://www.linkedin.com/in/alpha)", typ, val)
	}

	// C dropped (its canonical target collided with B).
	var countC int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+schema+`.person_identifiers WHERE id=$1`, idC).Scan(&countC); err != nil {
		t.Fatalf("count C: %v", err)
	}
	if countC != 0 {
		t.Errorf("row C count=%d, want 0 (should be dropped as a collision)", countC)
	}

	// B survivor intact.
	if err := pool.QueryRow(ctx, `SELECT identifier_type, identifier_value FROM `+schema+`.person_identifiers WHERE id=$1`, idB).Scan(&typ, &val); err != nil {
		t.Fatalf("scan B: %v", err)
	}
	if typ != "linkedin_url" || val != "https://www.linkedin.com/in/bravo" {
		t.Errorf("row B = (%q, %q), want unchanged (linkedin_url, https://www.linkedin.com/in/bravo)", typ, val)
	}

	// No legacy 'linkedin' rows remain.
	var legacyLeft int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+schema+`.person_identifiers WHERE identifier_type='linkedin'`).Scan(&legacyLeft); err != nil {
		t.Fatalf("count legacy: %v", err)
	}
	if legacyLeft != 0 {
		t.Errorf("legacy 'linkedin' rows remaining=%d, want 0", legacyLeft)
	}
}
