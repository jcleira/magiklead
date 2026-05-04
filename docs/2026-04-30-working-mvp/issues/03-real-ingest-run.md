# 03 — Run real ingest pipeline

**Type**: HITL — TeamPages seed list needs operator curation; ingest data quality should be reviewed before being shipped to production.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — runs locally before any deploy work

## What to build

Run the existing `cmd/ingest` pipeline end-to-end against the three free real-data sources — SEC EDGAR, Wikidata, TeamPages — locally against a Postgres instance that will later be migrated to production (issue #6). Run the email-verification batch afterwards so emails carry a real verification status.

After this slice, the canonical database contains real executive data: every officer of every US public company who filed a Form 3/4/5 in the last several quarters (EDGAR), notable global CEOs with Wikidata entries (Wikidata, deduped against EDGAR via SEC CIK), and execs from a curated seed list of company `/team` pages (TeamPages).

The TeamPages seed list — a list of company domains to scrape — is curated by the operator and committed as a config artefact so future runs reproduce.

## Acceptance criteria

- [x] Operator-curated TeamPages domain list committed to the repo (suggested location: `backend/cmd/ingest/teampages-domains.txt` — one domain per line).
- [x] EDGAR ingest run with a sensible since-date (e.g. last 4 quarters); `raw_ingests` table contains the resulting batches; resolver materialises canonical rows.
- [x] Wikidata ingest run against the live SPARQL endpoint; `raw_ingests` has a Wikidata batch; resolver dedupes Wikidata hits with EDGAR via SEC CIK.
- [x] TeamPages ingest run against the curated seed list; `raw_ingests` has a TeamPages batch; resolver materialises canonical rows.
- [x] Pipeline is architecturally ready to receive emails from any source: `internal/ai/extract.go` extracts `mailto:` hrefs and asks Claude for person-tied addresses, the TeamPages source forwards them as `fields["email"]`, and `internal/ingest/resolver.go` upserts them into the `emails` table. Email-verification batch (`backend/cmd/verify-emails`) runs cleanly against whatever emails accumulate. (Real-world public team pages yield zero person-tied emails on the curated B2B SaaS seed list — see issue #13 for the paid email-finder source that fills this gap.)
- [x] `POST /api/v1/leads/search` for "VP Sales" returns real persons (not fixtures); 120 current employments match the title fuzzy-search. Verified emails will surface once issue #13 lands.
- [x] Operator reviewed a sample of 50 random resolved persons and confirmed quality: no obvious duplicates the resolver missed, titles plausible, organisations correct. Two minor data-quality findings flagged (title-punctuation duplicate employment for one person; "See remarks" appearing as a literal title for two EDGAR rows where officers omitted a structured title).
- [x] Row counts logged for the post-PRD record: 29,685 persons; 6,350 organizations; 33,266 employments; 0 emails (pending issue #13); 5 raw_ingests sources (4 EDGAR quarterly batches, 1 Wikidata, 35 TeamPages); 108,212 source records.

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
- **Paid email-finder integration** — split out to [issue #13](./13-email-finder-source.md). The architectural wiring landed in this issue (extract → fields["email"] → resolver upsert) so the email-finder slice is purely a new ingest source plus a billing decision; no plumbing rework.
