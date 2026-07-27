// Local-dev fixture loader. Inserts a handful of canonical
// organizations + persons + employments — email prospects (with
// verified emails) and LinkedIn prospects (with a linkedin_url
// identifier, no email) — so the `/leads` search, save, and campaign
// flows return non-empty results on both channels in a fresh devpod.
// Idempotent — re-running clears prior fixture rows by canonical_name
// (cascade drops their identifiers + employments) and replants them.
//
// Invoke from the api container:
//
//	devpods exec api go run ./cmd/seed
package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/jcleira/magiklead/backend/internal/linkedin/liurl"
)

const seedSQL = `
BEGIN;

-- Wipe prior fixtures so the loader is idempotent. We key on a
-- recognisable suffix in canonical_name to avoid touching real ingest
-- data — the dev seeds all end with " (devpod-fixture)".
DELETE FROM persons       WHERE canonical_name LIKE '%(devpod-fixture)';
DELETE FROM organizations WHERE canonical_name LIKE '%(devpod-fixture)';

WITH org_input(canonical_name, primary_domain) AS (
    VALUES
        ('Acme Corp (devpod-fixture)',    'acme.test'),
        ('Globex Inc (devpod-fixture)',   'globex.test'),
        ('Initech (devpod-fixture)',      'initech.test'),
        ('Umbrella Co (devpod-fixture)',  'umbrella.test'),
        ('Soylent Ltd (devpod-fixture)',  'soylent.test')
), orgs AS (
    INSERT INTO organizations (canonical_name, primary_domain)
    SELECT canonical_name, primary_domain FROM org_input
    RETURNING id, canonical_name
), person_input(canonical_name, first_name, last_name, normalized_name, org_canonical, title, email_local) AS (
    VALUES
        ('Tim Cook (devpod-fixture)',      'Tim',     'Cook',      'cook tim',      'Acme Corp (devpod-fixture)',   'CEO',                 'tim'),
        ('Lisa Su (devpod-fixture)',       'Lisa',    'Su',        'lisa su',       'Acme Corp (devpod-fixture)',   'VP Engineering',      'lisa'),
        ('Jensen Huang (devpod-fixture)',  'Jensen',  'Huang',     'huang jensen',  'Acme Corp (devpod-fixture)',   'Head of Sales',       'jensen'),
        ('Reed Hastings (devpod-fixture)', 'Reed',    'Hastings',  'hastings reed', 'Acme Corp (devpod-fixture)',   'CRO',                 'reed'),
        ('Satya Nadella (devpod-fixture)', 'Satya',   'Nadella',   'nadella satya', 'Globex Inc (devpod-fixture)',  'CEO',                 'satya'),
        ('Sundar Pichai (devpod-fixture)', 'Sundar',  'Pichai',    'pichai sundar', 'Globex Inc (devpod-fixture)',  'VP Sales',            'sundar'),
        ('Marc Benioff (devpod-fixture)',  'Marc',    'Benioff',   'benioff marc',  'Globex Inc (devpod-fixture)',  'Head of Growth',      'marc'),
        ('Andy Jassy (devpod-fixture)',    'Andy',    'Jassy',     'jassy andy',    'Globex Inc (devpod-fixture)',  'Director of Sales',   'andy'),
        ('Susan Wojcicki (devpod-fixture)','Susan',   'Wojcicki',  'wojcicki susan','Initech (devpod-fixture)',     'CMO',                 'susan'),
        ('Eric Yuan (devpod-fixture)',     'Eric',    'Yuan',      'eric yuan',     'Initech (devpod-fixture)',     'Founder',             'eric'),
        ('Drew Houston (devpod-fixture)',  'Drew',    'Houston',   'houston drew',  'Initech (devpod-fixture)',     'CEO',                 'drew'),
        ('Stewart Butterfield (devpod-fixture)','Stewart','Butterfield','butterfield stewart','Initech (devpod-fixture)','Co-founder',  'stewart'),
        ('Patrick Collison (devpod-fixture)','Patrick','Collison','collison patrick','Umbrella Co (devpod-fixture)','CEO',               'patrick'),
        ('John Collison (devpod-fixture)', 'John',    'Collison',  'collison john', 'Umbrella Co (devpod-fixture)', 'President',           'john'),
        ('Brian Chesky (devpod-fixture)',  'Brian',   'Chesky',    'chesky brian',  'Umbrella Co (devpod-fixture)', 'CEO',                 'brian'),
        ('Daniel Ek (devpod-fixture)',     'Daniel',  'Ek',        'daniel ek',     'Umbrella Co (devpod-fixture)', 'Head of Product',     'daniel'),
        ('Jane Smith (devpod-fixture)',    'Jane',    'Smith',     'jane smith',    'Soylent Ltd (devpod-fixture)', 'VP Marketing',        'jane'),
        ('Bob Jones (devpod-fixture)',     'Bob',     'Jones',     'bob jones',     'Soylent Ltd (devpod-fixture)', 'Director of Product', 'bob'),
        ('Alice Wong (devpod-fixture)',    'Alice',   'Wong',      'alice wong',    'Soylent Ltd (devpod-fixture)', 'VP Sales',            'alice'),
        ('Carol Diaz (devpod-fixture)',    'Carol',   'Diaz',      'carol diaz',    'Soylent Ltd (devpod-fixture)', 'Head of Operations',  'carol')
), people AS (
    INSERT INTO persons (canonical_name, first_name, last_name, normalized_name)
    SELECT canonical_name, first_name, last_name, normalized_name FROM person_input
    RETURNING id, canonical_name
), employments AS (
    INSERT INTO employments (person_id, organization_id, title, is_current)
    SELECT p.id, o.id, pi.title, TRUE
    FROM person_input pi
    JOIN people p ON p.canonical_name = pi.canonical_name
    JOIN orgs   o ON o.canonical_name = pi.org_canonical
    RETURNING id
)
INSERT INTO emails (email, person_id, verification_method, verified_at, is_catchall, bounce_count)
SELECT
    pi.email_local || '@' || o.primary_domain,
    p.id,
    'fixture',
    NOW(),
    FALSE,
    0
-- primary_domain comes from the org_input VALUES CTE, NOT the
-- organizations table: those orgs were inserted by a data-modifying
-- CTE in this same statement, so a read of the table here sees the
-- pre-statement snapshot (no rows) and the join would yield nothing.
FROM person_input pi
JOIN people p ON p.canonical_name = pi.canonical_name
JOIN org_input o ON o.canonical_name = pi.org_canonical;

-- LinkedIn prospects. Same canonical shape as the email fixtures minus
-- the email — org → person → linkedin_url identifier → current
-- employment — so persons.has_email stays false. These are what the
-- LinkedIn-channel lead search returns (it keys on the linkedin_url
-- identifier and never requires an address). Synthetic, deterministic
-- linkedin_urls in the SyntheticURL(name, domain) shape. The DELETE at
-- the top of this transaction covers them too: dropping the person
-- cascades to its identifier + employment.
WITH li_org_input(canonical_name, primary_domain) AS (
    VALUES
        ('Hooli (devpod-fixture)',      'hooli.test'),
        ('Pied Piper (devpod-fixture)', 'piedpiper.test')
), li_orgs AS (
    INSERT INTO organizations (canonical_name, primary_domain)
    SELECT canonical_name, primary_domain FROM li_org_input
    RETURNING id, canonical_name
), li_person_input(canonical_name, first_name, last_name, normalized_name, org_canonical, title, linkedin_url) AS (
    VALUES
        ('Gavin Belson (devpod-fixture)',      'Gavin',   'Belson',    'gavin belson',      'Hooli (devpod-fixture)',      'CEO',                    'https://www.linkedin.com/in/gavin-belson-hooli'),
        ('Nelson Bighetti (devpod-fixture)',   'Nelson',  'Bighetti',  'nelson bighetti',   'Hooli (devpod-fixture)',      'Head of Product',        'https://www.linkedin.com/in/nelson-bighetti-hooli'),
        ('Richard Hendricks (devpod-fixture)', 'Richard', 'Hendricks', 'richard hendricks', 'Pied Piper (devpod-fixture)', 'Founder & CEO',          'https://www.linkedin.com/in/richard-hendricks-piedpiper'),
        ('Bertram Gilfoyle (devpod-fixture)',  'Bertram', 'Gilfoyle',  'bertram gilfoyle',  'Pied Piper (devpod-fixture)', 'Head of Infrastructure', 'https://www.linkedin.com/in/bertram-gilfoyle-piedpiper'),
        ('Dinesh Chugtai (devpod-fixture)',    'Dinesh',  'Chugtai',   'dinesh chugtai',    'Pied Piper (devpod-fixture)', 'Staff Engineer',         'https://www.linkedin.com/in/dinesh-chugtai-piedpiper'),
        ('Jared Dunn (devpod-fixture)',        'Jared',   'Dunn',      'jared dunn',        'Pied Piper (devpod-fixture)', 'Head of Operations',     'https://www.linkedin.com/in/jared-dunn-piedpiper')
), li_people AS (
    INSERT INTO persons (canonical_name, first_name, last_name, normalized_name)
    SELECT canonical_name, first_name, last_name, normalized_name FROM li_person_input
    RETURNING id, canonical_name
), li_employments AS (
    INSERT INTO employments (person_id, organization_id, title, is_current)
    SELECT p.id, o.id, li.title, TRUE
    FROM li_person_input li
    JOIN li_people p ON p.canonical_name = li.canonical_name
    JOIN li_orgs   o ON o.canonical_name = li.org_canonical
    RETURNING id
)
INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary)
SELECT p.id, 'linkedin_url', li.linkedin_url, TRUE
FROM li_person_input li
JOIN li_people p ON p.canonical_name = li.canonical_name;

COMMIT;
`

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal("connect: ", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		log.Fatal("seed exec: ", err)
	}

	// Report ACTUAL counts — a hardcoded message previously masked a bug
	// where 0 emails were inserted yet the seed still claimed success.
	var orgs, persons, emails, linkedin int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM organizations WHERE canonical_name LIKE '%(devpod-fixture)'),
		(SELECT count(*) FROM persons       WHERE canonical_name LIKE '%(devpod-fixture)'),
		(SELECT count(*) FROM emails         WHERE verification_method = 'fixture'),
		(SELECT count(*) FROM person_identifiers pi
		    JOIN persons p ON p.id = pi.person_id
		    WHERE pi.identifier_type = 'linkedin_url'
		      AND p.canonical_name LIKE '%(devpod-fixture)')`,
	).Scan(&orgs, &persons, &emails, &linkedin); err != nil {
		log.Fatal("seed count: ", err)
	}
	if emails == 0 || linkedin == 0 {
		log.Fatalf("seed: %d orgs + %d persons but %d emails / %d linkedin prospects — fixture is broken", orgs, persons, emails, linkedin)
	}

	// Every seeded linkedin_url must already be in liurl canonical form. The
	// fixture is the same canonical person graph the live search and the
	// ingest resolver write into, so a non-canonical literal here would
	// silently fork a prospect into two person rows (identifier_value is
	// globally UNIQUE and dedup matches the exact string). Validating against
	// the shared helper keeps the seed emitting canonical values via the same
	// single source of truth (issue #07).
	rows, err := pool.Query(ctx, `SELECT pi.identifier_value
		FROM person_identifiers pi
		JOIN persons p ON p.id = pi.person_id
		WHERE pi.identifier_type = 'linkedin_url'
		  AND p.canonical_name LIKE '%(devpod-fixture)'`)
	if err != nil {
		log.Fatal("seed canonical check: ", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			log.Fatal("seed canonical scan: ", err)
		}
		if c := liurl.Canonical(v); c != v {
			log.Fatalf("seed: fixture linkedin_url %q is not canonical (want %q) — align it with liurl.Canonical", v, c)
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal("seed canonical rows: ", err)
	}

	log.Printf("seed: %d orgs + %d persons + %d emails + %d linkedin prospects inserted", orgs, persons, emails, linkedin)
}
