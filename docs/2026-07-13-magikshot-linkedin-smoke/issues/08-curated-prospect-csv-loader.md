# 08 — Curated prospect CSV loader with planted/updated/skipped report

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [07 — LinkedIn identifier unification](./07-linkedin-identifier-unification.md)
(the loader writes through the unified type + canonical form and its
visibility ACs depend on the unified readers).

## What to build

A thin CLI (suggested: `backend/cmd/load-prospects/`) that takes the
founder-approved curated list as a CSV and plants it in the
canonical person graph so prospects appear in lead search like data
from any other source (PRD user stories 18, 19).

- **Input**: CSV with columns per the PRD — name, first name, last
  name, title, company, company domain, LinkedIn profile URL,
  location. This matches the existing
  `linkedinsearch.Profile{FullName, FirstName, LastName, Title,
  CompanyName, Domain, LinkedInURL, Location}` shape
  (`backend/internal/leads/linkedin_search/linkedin_search.go`)
  almost field-for-field.
- **Write path**: the existing canonical write-through —
  organization (by domain) → person → identifier → employment.
  Reuse `linkedinsearch.Module.WriteThrough` (already keyed on the
  reader-compatible `'linkedin_url'` identifier and org-by-domain)
  or the underlying `repository.Queries` upsert primitives
  (`CreateOrganization` / `CreatePerson` / `CreatePersonIdentifier`
  / `CreateEmployment` / `FindPersonByIdentifier` from
  `backend/queries/ingest.sql`), inside a transaction per the
  runner pattern. **No email rows** — these are LinkedIn prospects.
- **Validation + normalization**: per row — required fields
  present, domain sane, LinkedIn URL canonicalized via #07's shared
  helper; a bad row is rejected with a reason, and does not stop
  the run.
- **Idempotency**: upsert keyed on the canonical profile
  identifier; re-running the same file changes nothing and says so.
- **Report**: per-row outcome + summary counts of exactly what it
  **planted / updated / skipped** (and rejected, with reasons).
  Touches nothing outside its input.
- **Exec path**: runs inside the pod through the standard exec path
  (`devpods exec api go run ./cmd/load-prospects --file <path>` —
  the worktree is mounted, so a repo-relative path works). House
  CLI conventions: `godotenv.Load()`, stdlib `flag`, header comment
  documenting invocation like `cmd/seed`.
- Also commit: a documented CSV **template** (header + one example
  row) for the founder to fill in #09, and a test fixture CSV.

## Acceptance criteria

(All from the PRD's Testing Decisions — observe external behavior:
what the reader queries return and what the report says.)

- [ ] Integration test (in-pod, `//go:build integration`,
      `DATABASE_URL` guard): loading the fixture CSV lands the full
      graph — organizations, persons, `'linkedin_url'` identifiers
      in canonical form, employments.
- [ ] Running it again is idempotent: zero new rows, report says
      updated/skipped accordingly, counts match DB deltas exactly.
- [ ] A fixture with malformed rows (missing URL, bad domain, empty
      name): valid rows land, bad rows are rejected per-row with
      accurate reasons in the report.
- [ ] Loaded people are visible to the rail: returned by
      `ListLinkedInURLsForPersons` and selected by
      `GetDueLinkedInInviteLeads` / `GetDueLinkedInDMLeads` (reuse
      #07's visibility assertions).
- [ ] The CSV template is committed and documented.
- [ ] `devpods exec api go run ./cmd/load-prospects …` works
      against a devpod (manual smoke of the exec path).

## Modules touched

- `backend/cmd/load-prospects/` (new).
- Reuses `backend/internal/leads/linkedin_search/` write-through
  and/or `backend/internal/repository/` primitives; #07's helper.
- Fixture + template under the cmd's `testdata/` and this docs
  folder.

## Test prior art

- The Unipile connect-and-bind integration suite pattern
  (`backend/internal/handler/unipile_*_integration_test.go`): runs
  inside the pod against the live schema, skips without
  `DATABASE_URL`, cleans its own rows.
- Write-through integration test:
  `backend/internal/leads/linkedin_search/linkedin_search_test.go`.
- CLI conventions: `backend/cmd/seed`, `backend/cmd/ingest`.

## Out of scope

- The real curated list content and its approval —
  [#09](./09-curate-approve-load-list.md).
- Email enrichment / verification (LinkedIn-only prospects).
- Wiring the dormant RapidAPI live LinkedIn search (PRD out of
  scope).
