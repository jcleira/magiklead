// Local-dev fixture loader. Inserts a handful of canonical
// organizations + persons + employments + verified emails so the
// `/leads` search and save flows return non-empty results in a fresh
// devpod. Idempotent — re-running clears prior fixture rows by
// canonical_name and replants them.
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
    pi.email_local || '@' || o2.primary_domain,
    p.id,
    'fixture',
    NOW(),
    FALSE,
    0
FROM person_input pi
JOIN people p ON p.canonical_name = pi.canonical_name
JOIN org_input o ON o.canonical_name = pi.org_canonical
JOIN organizations o2 ON o2.canonical_name = o.canonical_name;

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

	if _, err := pool.Exec(context.Background(), seedSQL); err != nil {
		log.Fatal("seed exec: ", err)
	}
	log.Println("seed: 5 orgs + 20 persons inserted")
}
