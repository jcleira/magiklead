# 18 — Production data migration

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#16 — Hetzner VPS + api online](./16-hetzner-vps-and-api.md)

## What to build

Migrate the populated canonical database from the local
environment (where the working-MVP ingest run + any subsequent
PDL write-throughs in Phase 5 landed) to the production Hetzner
Postgres. This avoids re-running a multi-hour ingest in production
and gives the production quality gate real data on day one.

Devpod fixture rows (those with the `(devpod-fixture)` suffix)
are filtered out — production is real data only.

PDL-written-through rows from Phase 5 do come across — they're
real data, just inserted via the live-search pathway rather than
batch ingest. The `verification_method='pdl-verified'` rows are
preserved.

## Acceptance criteria

- [ ] `pg_dump` of local Postgres after the working-MVP ingest +
      Phase 5 PDL writes, with fixture rows excluded
      (`--exclude-table-data` or a `WHERE` filter via
      `--data-only` + custom `INSERT` script). Compressed with
      `gzip` or `pg_dump`'s custom format.
- [ ] Dump transferred to the Hetzner VPS via scp.
- [ ] Restored into production Postgres using `pg_restore`
      (custom format) or `psql < dump.sql` (plain SQL).
- [ ] `SELECT COUNT(*) FROM persons` on production matches the
      local non-fixture count.
- [ ] `SELECT COUNT(*) FROM emails WHERE verification_method != 'fixture'`
      on production matches the local count.
- [ ] `SELECT COUNT(*) FROM emails WHERE verification_method = 'pdl-verified'`
      on production is non-zero (Phase 5 wrote at least some
      PDL-verified rows).
- [ ] No `(devpod-fixture)` rows present on production:
      `SELECT COUNT(*) FROM persons WHERE canonical_name LIKE '%(devpod-fixture)'`
      returns 0.
- [ ] `https://api.magiklead.com/api/v1/leads/search` (with a
      valid auth token) for "VP Sales" returns the same set of
      canonical persons as the local environment.

## Modules touched

- Operational only — no code changes.

## Test prior art

- None. Verification is by row counts and a sample search query.

## Out of scope

- Backups of the migrated data — see
  [#23](./23-postgres-backups.md).
- Future incremental ingest in production — out of scope per PRD;
  v1 is a one-shot run.
- Object storage migration — `raw_ingests` blobs in MinIO/S3 are
  not migrated; the canonical tables are sufficient. If the audit
  trail matters in production, that's a separate task.
- Per-tenant data migration — there are no tenants on production
  yet; the first sign-ups in Phase 8 create their own.
