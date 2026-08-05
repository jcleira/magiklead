# 07 — LinkedIn identifier unification: one type, one canonical URL form

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately.

## What to build

Make "LinkedIn prospect" mean the same thing everywhere (PRD user
story 20): every writer of LinkedIn person identity and every reader
converge on **one identifier type storing one canonical full profile
URL**, with a data migration for legacy rows. Today the divergence
is two-level:

- **Type.** The ingest pipeline writes `person_identifiers` rows
  with `identifier_type = 'linkedin'`
  (`identifierLinkedIn`, `backend/internal/ingest/resolver.go`),
  while every rail reader filters exclusively on `'linkedin_url'`:
  the lead-search attach `ListLinkedInURLsForPersons`
  (`backend/queries/linkedin_search.sql`) and the due-lead queries
  `GetDueLinkedInInviteLeads` / `GetDueLinkedInDMLeads`
  (`backend/queries/campaign_leads.sql`). Ingest-created people are
  therefore invisible to LinkedIn search and never selected for
  sends. `backend/internal/handler/privacy.go` also keys
  erasure/blocklist lookups on `'linkedin'`
  (`identifierTypeLinkedIn`) — converge it too, or erasure misses
  unified rows.
- **Value format.** Ingest normalizes to scheme-less
  `linkedin.com/in/<slug>` (`NormalizeLinkedInURL`); `cmd/seed` and
  the live-search write-through
  (`backend/internal/leads/linkedin_search/linkedin_search.go`,
  type constant `IdentifierType = "linkedin_url"`) store full
  `https://www.linkedin.com/in/<slug>` URLs. Because
  `person_identifiers.identifier_value` is globally UNIQUE and
  dedup matches on exact `(type, value)`
  (`FindPersonByIdentifier`, `backend/queries/ingest.sql`), the two
  spellings never collide — the same human planted by two sources
  yields two person rows.

The PRD decides the canonical form is the **full canonical profile
URL**. Build:

1. **Shared normalization helper** — a single small package (e.g. a
   sibling under `backend/internal/linkedin/`) that maps any messy
   input (scheme variants, missing scheme, `www.`/mobile hosts,
   trailing slash, query strings/fragments, uppercase slugs, bare
   `/in/<slug>` paths) to the one canonical URL form, and is the
   only place that form is defined. Document the chosen shape
   (e.g. `https://www.linkedin.com/in/<slug>`) in the package doc.
2. **Converge writers** on `'linkedin_url'` + helper output: the
   ingest resolver's `personIdentifiers()`, `cmd/seed`'s fixture
   INSERT, and the linkedin_search write-through (currently raw
   `TrimSpace`; `SyntheticURL` output must already be
   canonical-form). The curated loader (#08) will use the same
   helper. Keep `linkedin_member_id` (send-time Unipile cache,
   `backend/internal/worker/linkedin_resolve.go`) untouched —
   different concern.
3. **Data migration** (next `backend/migrations/NNN` pair):
   rewrite existing `person_identifiers` rows with
   `identifier_type = 'linkedin'` to `'linkedin_url'` +
   canonicalized value. Handle the UNIQUE collision: if the
   canonical target value already exists on another row, keep the
   existing `'linkedin_url'` row and drop the legacy duplicate
   (person-row merging is out of scope). Prior art for DML
   migrations: `014_normalized_name.up.sql` (in-place value
   backfill), `016_tenant_leads.up.sql` (INSERT…SELECT +
   UPDATE…FROM). Decide and document the down migration's stance
   (type flip back without de-normalizing values is acceptable —
   the rewrite is lossy).
4. Readers stay as they are (`'linkedin_url'`), and the Unipile
   slug extractor (`publicIdentifierFromURL`,
   `backend/internal/linkedin/unipile/unipile.go`) already
   tolerates both spellings — selection, not parsing, is what's
   broken today.

## Acceptance criteria

- [x] Helper unit tests cover the messy-input matrix from the PRD's
      Testing Decisions: scheme variants, trailing slashes, query
      strings, uppercase slugs — plus no-scheme and bare-path
      inputs; all map to one canonical string.
- [x] Ingest resolver writes `'linkedin_url'` + canonical value;
      `TestPersonIdentifiers` and `TestNormalizeLinkedInURL`
      (`backend/internal/ingest/resolver_test.go`) updated — they
      currently pin the divergent behavior.
- [x] `cmd/seed` and the linkedin_search write-through emit
      canonical values via the same helper
      (`TestSyntheticURL` in
      `backend/internal/leads/linkedin_search/` updated if the
      canonical form differs).
- [x] `privacy.go` erasure/blocklist keying works against unified
      rows.
- [x] Migration test over legacy-shaped rows: `'linkedin'` rows are
      rewritten to type + canonical value; a legacy row whose
      canonical value collides with an existing row is dropped, the
      survivor intact.
- [x] Cross-source visibility (integration tests, in-pod): a person
      planted through the ingest resolver is returned by
      `ListLinkedInURLsForPersons` AND selected by
      `GetDueLinkedInInviteLeads` and `GetDueLinkedInDMLeads`.
- [x] No writer of person LinkedIn identity under any type other
      than `'linkedin_url'` remains (grep-clean), and
      `go test ./...` + `-tags=integration` pass in the pod.

## Modules touched

- New helper package under `backend/internal/linkedin/`.
- `backend/internal/ingest/resolver.go` (+ tests).
- `backend/cmd/seed/main.go`.
- `backend/internal/leads/linkedin_search/linkedin_search.go`.
- `backend/internal/handler/privacy.go`.
- `backend/migrations/` (new pair). Reader queries unchanged — no
  sqlc regeneration expected.

## Test prior art

- Normalization unit tests: `backend/internal/ingest/resolver_test.go`.
- In-pod integration pattern (`//go:build integration`,
  `DATABASE_URL` skip guard, `t.Cleanup` row deletes):
  `backend/internal/handler/unipile_*_integration_test.go`; run via
  `devpods exec api go test -tags=integration ./...`.
- Write-through assertions:
  `backend/internal/leads/linkedin_search/linkedin_search_test.go`.

## Out of scope

- Merging duplicate person rows created by the historical format
  split — the migration only resolves identifier rows.
- `organization_identifiers` LinkedIn rows (ingest's org-level
  `'linkedin'` type has no rail reader).
- Teaching the PDL path to extract LinkedIn URLs (PDL writes
  `pdl_id` only; not a LinkedIn-identity writer per the PRD).
- The `linkedin_member_id` cache.
