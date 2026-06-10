# 02 — Source LinkedIn prospects into the canonical

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A tenant searches by ICP (titles, industries, company size, locations,
free-text) and gets back LinkedIn prospects — name, title, company,
LinkedIn URL — with no email anywhere. Each result writes through to the
canonical persons graph keyed on a `linkedin_url` identifier, so the
same prospect is cached across tenants (mirrors PDL write-through, minus
the email).

End-to-end:
- New `leads/linkedin_search` source behind a `Doer` HTTP seam:
  production = a RapidAPI LinkedIn people-search (default rockapis
  `linkedin-data-api`, swappable); demo mode = the in-tree
  `internal/leads/scraper.go` (company-site scrape) feeding the SAME
  write-through, so no external key is needed to demo.
- Write-through mirrors the PDL chain: upsert organization (by
  primary_domain), upsert person (dedup by a `person_identifiers` row of
  type `linkedin_url`), create the `linkedin_url` identifier, upsert
  current employment. Writes NO `emails` row; `has_email` stays false.
- Wired into the existing `POST /api/v1/leads/search`; LinkedIn results
  render in the leads UI with a profile link and no email column.

## Acceptance criteria

- [ ] New `FindPersonByIdentifier(identifier_type, identifier_value)`
  query (the PDL path hard-codes `pdl_id`); used to dedup on
  `linkedin_url`.
- [ ] `leads/linkedin_search` behind a `Doer` seam; a stubbed response
  of N profiles writes through N persons + N `linkedin_url` identifiers
  + their orgs/employments; assert via DB.
- [ ] Re-ingesting the same LinkedIn URL updates in place — one person,
  one identifier (unique on `person_identifiers.identifier_value`);
  assert no duplicate rows.
- [ ] No `emails` row is ever created by this path; assert via DB.
- [ ] Demo mode (scraper.go source) produces canonical persons with
  `linkedin_url` identifiers using only HTTP to company sites — no
  RapidAPI key; runnable in the devpod (a small `cmd/` or extend
  `cmd/seed`).
- [ ] `POST /api/v1/leads/search` returns LinkedIn-sourced prospects
  (name/title/company/linkedin_url); `with_email` is not required.
- [ ] Leads UI lists LinkedIn prospects with a profile link, no email
  column.

## Modules touched

- New `internal/leads/linkedin_search` — RapidAPI seam + write-through
  (mirror `internal/leads/pdl` `writeThrough`/upsert chain, minus the
  email upsert).
- `internal/leads/scraper.go` — reused as the demo-mode people source
  feeding the same write-through.
- Repository: `FindPersonByIdentifier`; reuse the PDL upsert queries (or
  generalize their names away from `FromPDL`).
- `internal/handler/leads_search.go` — source selection (canonical-first
  already exists; the LinkedIn source populates it).
- Frontend leads list — LinkedIn prospect rendering.

## Test prior art

- `internal/leads/pdl/*_test.go` and the PDL `wire_internal_test.go` —
  write-through + dedup assertions against a stub.
- The canonical schema migrations + the `lead_search` queries.

## Out of scope

- Connecting an account ([01](./01-connect-linkedin-account.md)) and any
  sending ([04](./04-send-connection-invite.md)+).
- Choosing/buying the production RapidAPI key — demo mode needs none;
  prod is a config swap behind the seam (does not block #3, which only
  needs canonical prospects that demo mode produces).
