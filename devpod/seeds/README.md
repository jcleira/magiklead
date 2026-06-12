# Devpod seed snapshot — lifecycle

This directory hosts the seed-snapshot regeneration script consumed by
[devpods](https://github.com/jcleira/devpods). The actual archive
(`db.sql.gz` + `manifest.json`) lives in devpods' cache, not in this
repo — `regenerate.sh` writes to `$SEEDS_DIR` which devpods sets.

## What the snapshot is

A point-in-time `pg_dump` of the devpod's Postgres database, taken
after the in-tree fixture loader (`backend/cmd/seed`) has run on top
of fresh migrations. When a devpod is destroyed and re-created,
devpods replays this snapshot rather than re-running the loader, so
canonical-graph data (`persons`, `organizations`, `emails`, etc.) is
restored without a network round-trip to the live ingest sources.

## The rule: regenerate after any data-changing operation

**If you mutate canonical data, regenerate the snapshot before
signing the work off.** The snapshot is the only persistence of that
data across devpod rebuilds. Without a regeneration step, the work
evaporates the next time the devpod is destroyed.

Data-changing operations include:

- Running an ingest source against live data (e.g.
  `cmd/ingest sec-edgar`, `cmd/ingest wikidata`,
  `cmd/ingest teampages`).
- Bulk-loading fixtures via `cmd/seed` or any ad-hoc loader.
- Running a schema migration that backfills rows (a column rewrite,
  a constraint cleanup, an enum value rename).
- Any `psql` session that inserts / updates / deletes more than a
  handful of rows.

After the mutation completes, run:

```
devpods seed regenerate
```

This invokes `regenerate.sh` inside the devpod: re-runs the fixture
loader, dumps the resulting database, and updates `manifest.json`
with the current migration hash.

Verify the rule was honoured before closing the work item: the
snapshot's `manifest.json` `created_at` timestamp should post-date
the mutation.

## Why this is a rule and not a convention

On 2026-05-07 the working-MVP smoke walk lost 29k+ canonical persons
when the devpod was destroyed; the snapshot pre-dated the ingest
run, so `devpods up` replayed an older state and the ingest output
was unrecoverable without re-running the sources. The rule above is
the fix.

## What does NOT need regeneration

- Application-state changes that the loader will recreate on its own
  next time (signed-up users, OAuth tokens, sequences, campaigns —
  anything tenant-scoped).
- Test artefacts in MinIO / S3 (handled separately by the storage
  module's own lifecycle).
- Code-only changes (handlers, migrations without backfill,
  middleware, frontend) — these flow through migrations on the next
  `devpods up`.

## Related

- `regenerate.sh` — the script `devpods seed regenerate` invokes.
- `backend/cmd/seed` — the in-tree fixture loader. Idempotent;
  clears prior `(devpod-fixture)` rows and replants them.
- `../../CLAUDE.md` — devpod command reference.
