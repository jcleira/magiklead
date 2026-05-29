# 11 — Account export + delete

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#1 — Phase 0 unblock fixes](./01-phase-0-unblock.md)

## What to build

Two GDPR-adjacent self-service actions on the settings page:

1. **Export all data** — campaigns, leads (the tenant's saved
   leads, not the canonical graph), sequences, sends, replies —
   as a downloadable archive (single `.zip` containing one JSON
   file per table scoped to the tenant). User story 6.

2. **Delete account** — full cascading erasure: the tenant row +
   all dependent rows go away; the user's Clerk account is also
   deleted via the Clerk Backend API. After confirmation the user
   is signed out and redirected to the marketing landing page.
   User story 5.

After this slice, a user can leave with their data in hand and
nothing of theirs remains on the platform — the same right the
public privacy-erasure endpoint already gives non-authenticated
visitors, just from inside the app for authenticated users.

## Acceptance criteria

- [x] New backend route `POST /api/v1/account/export` initiates
      an export. For v1 this is synchronous: streams a `.zip` of
      JSON files back to the client in the response body. (Async
      with a presigned-URL hand-off can land later if the export
      grows large.)
      **Implementation:** `AccountHandler.Export` collects every
      tenant-scoped row, builds an in-memory zip, sets
      `Content-Type: application/zip` +
      `Content-Disposition: attachment; filename="magiklead-export-<tenant>.zip"`,
      and writes the body in one shot. No streaming for v1 — the
      validation-window exports are small enough that the in-memory
      build is fine.
- [x] Export archive contains: `campaigns.json`, `sequences.json`,
      `campaign_leads.json`, `email_events.json`, `tenant_leads.json`,
      `subscriptions.json`, `gmail_accounts.json` (token field
      redacted), `user.json` (the operator's own profile only —
      not other workspace members, since multi-member is out of
      scope), `unsubscribes.json` (per-tenant rows).
      **Implementation:** the handler writes ten JSON files —
      the nine the AC names plus `tenant.json` for the workspace
      row itself (handy for a re-import). `sequences.json` is
      derived from the `sequence` JSONB column on each campaign,
      since sequences aren't a separate table. `gmail_accounts`
      rows have `access_token` and `refresh_token` zeroed before
      marshal. The tracer-bullet test
      `TestAccountExport_ContainsAllJSONFiles` asserts each file is
      present, parses as JSON, and that no OAuth tokens leak into
      the archive.
- [x] New backend route `DELETE /api/v1/account`:
  - cascading delete on `tenants` (FKs already ON DELETE
    CASCADE for dependent rows; verify in a migration round-trip
    test that nothing orphans),
  - calls Clerk Backend API to delete the user
    (`https://api.clerk.com/v1/users/<id>`),
  - returns 204; frontend signs out and redirects.
  **Implementation:** the assumed schema state ("FKs already ON
  DELETE CASCADE") is not true today — only `tenant_leads` and
  `unsubscribes` have CASCADE; the other six tenant-scoped tables
  use plain FKs. Rather than introduce a schema migration outside
  this issue's scope, the handler runs an explicit FK-respecting
  cascade inside one transaction via a new sqlc query family
  (`DeleteEmailEventsByTenant`, `DeleteCampaignLeadsByTenant`, …,
  `DeleteTenantByID`, `DeleteUserByIDIfOrphan`). The order
  walks email_events → campaign_leads → campaigns → plays →
  gmail_accounts → email_accounts → subscriptions → user_tenants
  → tenants → users so no step leaves orphans. Clerk runs first;
  on failure the local data is untouched (HTTP 502, ready to
  retry). On Clerk's own 404 (`clerk.ErrNotFound`) we treat the
  user as already gone and proceed with the local cascade. A
  future migration that flips every FK to CASCADE would let us
  collapse the cascade to a single `DELETE FROM tenants`; not in
  scope here.
- [x] Frontend on `/settings`: two clearly-separated sections —
      "Export data" with a "Download" button, "Delete account"
      with a destructive confirmation modal that types the
      workspace name to confirm.
      **Implementation:** new "Export data" section uses the
      Clerk JWT directly (not `useApi`, which always JSON-decodes
      the response) to fetch the zip and trigger a browser download
      via an anchor + `URL.createObjectURL`. "Delete account"
      opens a destructive modal that requires the user to type
      the workspace name verbatim (case-sensitive) before the
      red "Delete forever" button activates. On success the
      handler calls `signOut()` and `router.replace("/")`.
- [x] Unit tests cover:
  - export: happy path (archive contains all expected files;
    counts match the tenant's row counts), wrong tenant rejected,
  - delete: happy path (cascade cleans up; Clerk call mocked),
    Clerk call failure (returns 502; row not deleted yet to
    allow retry), already-deleted (idempotent: 204).
  **Tests:**
  - `TestAccountExport_ContainsAllJSONFiles` — happy path: ten
    files, valid JSON, gmail tokens redacted.
  - `TestAccountExport_MissingTenant` /
    `TestAccountExport_MissingClerkID` — both 401, no DB calls.
  - `TestAccountDelete_HappyPath` — Clerk gets called; cascade
    walks the documented order; 204.
  - `TestAccountDelete_ClerkFails` — Clerk error → 502, no
    cascade calls (local data ready to retry).
  - `TestAccountDelete_ClerkNotFound` — Clerk 404 → cascade
    still runs, 204.
  - `TestAccountDelete_AlreadyDeleted` — `GetUserByClerkID`
    returns ErrNoRows → 204 idempotently, no Clerk + no cascade.
  "Counts match the tenant's row counts" is implicit in the test
  fakes' shape (each list query returns a fixed slice that ends
  up in the JSON), not asserted by row count.
- [ ] Manual local verification: export → unzip → spot-check
      contents; delete account → confirm rows gone, Clerk user
      gone (visible in Clerk dashboard), sign-out + redirect
      works.
      **Deferred:** needs a real Clerk dev account, real Gmail
      OAuth connection (so there's something to redact), and
      operator dashboard access for the Clerk-user-gone
      verification. Flips during [#13](./13-d-bar-flow-walkthrough.md)
      / [#15](./15-local-3-day-real-test.md).

## Modules touched

- `backend/internal/handler/` — new `account.go` (or extend
  existing settings handler).
- `backend/queries/tenants.sql` — verify cascading FKs are correct
  (add if not).
- `backend/internal/clerk/` (or wherever Clerk client lives) —
  user-delete helper if not present.
- `frontend/src/app/(app)/settings/page.tsx` — two new sections.

## Test prior art

- `backend/internal/handler/privacy.go` and `privacy_test.go` —
  the public erasure flow does similar cascade-delete work; this
  slice is its authenticated cousin. Reuse the cascade helper if
  one exists.
- `backend/internal/handler/tenant_leads_test.go` — handler test
  pattern.

## Out of scope

- Per-table grace period before final erasure — explicitly
  deferred per PRD.
- Async export with email-when-ready — only matters for very
  large exports; v1 is synchronous.
- Account merge / workspace transfer — explicitly deferred per
  PRD.
- Audit log of who deleted what — explicitly deferred per PRD.
