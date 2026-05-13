# 07 — PDL integration + canonical write-through

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#1 — Phase 0 unblock fixes](./01-phase-0-unblock.md)

## What to build

Wire People Data Labs ($98/mo, 5k credits/mo entry plan) as the
real-data spine behind the existing search engine. The existing
canonical graph (EDGAR + Wikidata + TeamPages, ~29k persons after
the working-MVP ingest run) sits *underneath* as a free cache: a
user's search hits the canonical first; if the canonical is empty
or stale for the query, PDL is called; PDL results are then
written back to `persons` + `emails` so subsequent identical
queries across any tenant hit the cache for free.

Layer in the missing filter dimensions that the search handler
today rejects with `501 filter_unsupported`: `industries`,
`company_size`, `locations`. Add a free-text ICP description field
that the onboarding flow already produces — PDL's person-search
API takes both structured filters and a free-text "Elasticsearch
query" form, so both shapes have a real backend.

After this slice, a tester pasting their website in onboarding gets
plausible buyer ICPs; refining the ICP with free-text or
structured filters returns real B2B persons (~1.5B universe) with
SMTP-verified emails; every saved lead counts against the tenant's
monthly quota; identical lookups across tenants don't re-charge
PDL credits.

## Acceptance criteria

- [ ] Provider account exists; `PDL_API_KEY` in
      `~/.config/devpods/magiklead/.env.backend`; api startup
      tolerates missing key in dev (falls through to canonical-only
      mode) but logs a clear warning so the operator knows enrichment
      is off.
- [ ] New deep module `backend/internal/leads/pdl/`:
  - `Search(ctx, filters) ([]Person, error)` — uses PDL's
    person-search API with the structured filters and/or
    free-text ICP description.
  - `Enrich(ctx, person) (Email, error)` — used when search
    returned a person without a verified email; calls PDL's
    person-enrichment API.
  - Both write through to canonical: `Search` upserts every
    returned person into `persons` + `employments`, every
    organisation into `organizations`, every verified email into
    `emails` with `verification_method='pdl-verified'` and
    `verified_at=NOW()`.
  - 90-day staleness: if a canonical row was last touched >90d
    ago, treat as a cache miss and re-call PDL.
  - Stubbable HTTP-client seam (`Doer`) for testing.
- [ ] `POST /api/v1/leads/search` handler extended:
  - `industries`, `company_size`, `locations` filters now flow
    through (no longer return 501),
  - new free-text `description` field on the request,
  - on a canonical miss (no results, or all results stale), the
    handler calls `leads/pdl.Search(...)` synchronously, awaits
    the write-through, then re-queries canonical and returns the
    result. SLA: P95 < 3s for a PDL-backed search.
- [ ] `POST /api/v1/leads/save` (or whatever the existing save
      route is named) increments the tenant's monthly saved-leads
      count exactly once per unique person across all
      campaigns/sequences. Multi-step sends to a single saved lead
      cost one quota unit, not many.
- [ ] Frontend extends the search UI with:
  - a free-text ICP description input (carries over from onboarding
    if present),
  - structured filter controls for industries, company size,
    locations,
  - clear "verified email" / "unverified" badges on each result
    row.
- [ ] Unit tests for `leads/pdl`:
  - happy path: search returns N persons, all written to canonical,
  - cache hit: canonical has all results within 90 days — PDL not
    called,
  - stale cache: canonical row's `updated_at` >90 days — PDL
    re-called, canonical updated,
  - failure paths: PDL rate limit (429), malformed response,
    credit exhausted (402), auth failure (401).
- [ ] Skip-pattern integration test (`PDL_API_KEY` env var present
      → run, absent → skip) that does a single live `Search` against
      PDL and asserts non-empty results + non-zero credits consumed
      (logged for operator cost tracking).
- [ ] Manual local verification: operator pastes magikshot.com into
      onboarding → AI plays generated → search with "VP Marketing"
      returns real persons → save 5 leads → quota counter shows 5/100.

## Modules touched

- New deep module: `backend/internal/leads/pdl/`.
- `backend/internal/handler/leads_search.go` — extended to accept
  new filters + free-text description; call PDL on canonical miss
  + stale.
- `backend/queries/lead_searches.sql` and
  `backend/queries/persons.sql` — extended for new filter columns
  if schema additions are needed (industries, company size, location
  may need new columns on `organizations` / `persons`).
- `frontend/src/app/(app)/leads/page.tsx` — search UI extensions.
- `frontend/src/app/(app)/onboarding/page.tsx` — wires
  ICP-description into the saved campaign so the search inherits it.

## Test prior art

- `backend/internal/ingest/sources/edgar_test.go`,
  `wikidata_test.go` — HTTP-mocked third-party API tests.
- `backend/internal/leads/smtp_verify.go` + existing
  `cmd/verify-emails` — pattern for an enrichment module that
  iterates persons and writes back through queries.
- `backend/internal/storage/s3_roundtrip_test.go` — skip-pattern
  for integration tests that need a real external service.

## Out of scope

- Multi-provider fallback (Findymail, Hunter.io as backups) —
  start with PDL only; add fallback only if coverage on the actual
  list turns out poor in Phase 5's 3-day real test.
- Phone-number enrichment — PDL offers phones; v1 scope is email.
- LinkedIn-URL → email enrichment as a primary lookup — PDL accepts
  LinkedIn URL as an input; this slice does not surface that input
  in the UI.
- Continuous re-enrichment as a scheduled job — v1 is "lookup on
  search-time miss"; standing job can land later.
- Crunchbase ingest — explicitly out of scope per PRD.
- Note: the working-MVP issue
  [#14 — Email finder source](../../2026-04-30-working-mvp/issues/14-email-finder-source.md)
  recommended Findymail. This PRD supersedes that call in favour of
  PDL (per the PRD's lead-discovery decision). The operator can
  swap providers later if Phase 5 surfaces coverage issues.
