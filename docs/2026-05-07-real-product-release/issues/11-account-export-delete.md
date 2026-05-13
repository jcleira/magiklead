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

- [ ] New backend route `POST /api/v1/account/export` initiates
      an export. For v1 this is synchronous: streams a `.zip` of
      JSON files back to the client in the response body. (Async
      with a presigned-URL hand-off can land later if the export
      grows large.)
- [ ] Export archive contains: `campaigns.json`, `sequences.json`,
      `campaign_leads.json`, `email_events.json`, `tenant_leads.json`,
      `subscriptions.json`, `gmail_accounts.json` (token field
      redacted), `user.json` (the operator's own profile only —
      not other workspace members, since multi-member is out of
      scope), `unsubscribes.json` (per-tenant rows).
- [ ] New backend route `DELETE /api/v1/account`:
  - cascading delete on `tenants` (FKs already ON DELETE
    CASCADE for dependent rows; verify in a migration round-trip
    test that nothing orphans),
  - calls Clerk Backend API to delete the user
    (`https://api.clerk.com/v1/users/<id>`),
  - returns 204; frontend signs out and redirects.
- [ ] Frontend on `/settings`: two clearly-separated sections —
      "Export data" with a "Download" button, "Delete account"
      with a destructive confirmation modal that types the
      workspace name to confirm.
- [ ] Unit tests cover:
  - export: happy path (archive contains all expected files;
    counts match the tenant's row counts), wrong tenant rejected,
  - delete: happy path (cascade cleans up; Clerk call mocked),
    Clerk call failure (returns 502; row not deleted yet to
    allow retry), already-deleted (idempotent: 204).
- [ ] Manual local verification: export → unzip → spot-check
      contents; delete account → confirm rows gone, Clerk user
      gone (visible in Clerk dashboard), sign-out + redirect
      works.

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
