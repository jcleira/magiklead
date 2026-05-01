# 03 — Run real ingest pipeline

**Type**: HITL — TeamPages seed list needs operator curation; ingest data quality should be reviewed before being shipped to production.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — runs locally before any deploy work

## What to build

Run the existing `cmd/ingest` pipeline end-to-end against the three free real-data sources — SEC EDGAR, Wikidata, TeamPages — locally against a Postgres instance that will later be migrated to production (issue #6). Run the email-verification batch afterwards so emails carry a real verification status.

After this slice, the canonical database contains real executive data: every officer of every US public company who filed a Form 3/4/5 in the last several quarters (EDGAR), notable global CEOs with Wikidata entries (Wikidata, deduped against EDGAR via SEC CIK), and execs from a curated seed list of company `/team` pages (TeamPages).

The TeamPages seed list — a list of company domains to scrape — is curated by the operator and committed as a config artefact so future runs reproduce.

## Acceptance criteria

- [ ] Operator-curated TeamPages domain list committed to the repo (suggested location: `backend/cmd/ingest/teampages-domains.txt` — one domain per line).
- [ ] EDGAR ingest run with a sensible since-date (e.g. last 4 quarters); `raw_ingests` table contains the resulting batches; resolver materialises canonical rows.
- [ ] Wikidata ingest run against the live SPARQL endpoint; `raw_ingests` has a Wikidata batch; resolver dedupes Wikidata hits with EDGAR via SEC CIK.
- [ ] TeamPages ingest run against the curated seed list; `raw_ingests` has a TeamPages batch; resolver materialises canonical rows.
- [ ] Email-verification batch (`backend/cmd/verify-emails`) runs against the new emails; `emails.verification_method` reflects the verifier's outcome (verified / catchall / unverifiable).
- [ ] `POST /api/v1/leads/search` for "VP Sales" returns real persons (not fixtures) with verified emails surfaced.
- [ ] Operator reviews a sample of resolved persons (suggest 50 random rows) and confirms quality: no obvious duplicates the resolver missed, titles look plausible, organisations correct.
- [ ] Row counts logged for the post-PRD record: `persons`, `organizations`, `employments`, `emails`, plus a verification-status breakdown.

## Modules touched

- Ingest CLI (`backend/cmd/ingest`).
- Email verification CLI (`backend/cmd/verify-emails`).
- TeamPages seed list (new committed config artefact).

## Test prior art

- `backend/internal/ingest/sources/*_test.go` — unit tests for each source already exist. This issue is operational; verification is by query and operator inspection.

## Out of scope

- CrunchBase ingest — explicitly skipped per PRD; the source code stays in the repo for future use.
- Production data migration — see [issue #6](./06-production-data-migration.md).
- Recurring scheduled ingest in production — out of scope; v1 is a one-shot run.
- Operator-driven source-by-source quality reviews after each step (acceptable to bundle the whole review at the end).
