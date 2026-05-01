# 06 — Production data migration

**Type**: AFK — mechanical `pg_dump` + scp + restore once #3 and #4 are done.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#3 — Run real ingest pipeline](./03-real-ingest-run.md), [#4 — Hetzner VPS + domain + api online](./04-hetzner-vps-and-api.md)

## What to build

Migrate the populated canonical database from the local environment (where issue #3's ingest ran) to the production Hetzner Postgres. This avoids re-running a multi-hour ingest in production and gives the production smoke walkthrough real data on day one.

Devpod fixture rows (those with the `(devpod-fixture)` suffix) are filtered out — production is real data only.

## Acceptance criteria

- [ ] `pg_dump` of local Postgres after the ingest run, with fixture rows excluded (e.g. `--exclude-table-data` or a `WHERE` filter via `--data-only` + custom `INSERT` script). Compressed with `gzip` or `pg_dump`'s custom format.
- [ ] Dump transferred to the Hetzner VPS via scp.
- [ ] Restored into production Postgres using `pg_restore` (custom format) or `psql < dump.sql` (plain SQL).
- [ ] `SELECT COUNT(*) FROM persons` on production matches the local non-fixture count.
- [ ] `SELECT COUNT(*) FROM emails WHERE verification_method != 'fixture'` on production matches the local count.
- [ ] No `(devpod-fixture)` rows present on production: `SELECT COUNT(*) FROM persons WHERE canonical_name LIKE '%(devpod-fixture)'` returns 0.
- [ ] `https://api.<domain>/api/v1/leads/search` (with a valid auth token) for "VP Sales" returns the same set of canonical persons as the local environment.

## Modules touched

- Operational only — no code changes.

## Test prior art

- None. Verification is by row counts and a sample search query.

## Out of scope

- Backups of the migrated data — see [issue #11](./11-postgres-backups.md).
- Future incremental ingest in production — out of scope per PRD; v1 is a one-shot run.
- Object storage migration — `raw_ingests` blobs in MinIO/S3 are not migrated; the canonical tables are sufficient for the smoke path. If the audit trail matters in production, that's a separate task.
