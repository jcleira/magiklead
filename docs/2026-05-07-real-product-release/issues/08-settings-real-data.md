# 08 — Settings page real data

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#3 — Gmail OAuth real send](./03-gmail-oauth-send.md)

## What to build

Replace the hard-coded "Free" plan label and "100 leads/mo" usage
string on `frontend/src/app/(app)/settings/page.tsx` with real data
from the api: workspace name, plan, current monthly usage,
connected email accounts (with per-mailbox status indicators), and
controls to disconnect a Gmail account and reconnect a different
one.

This is the single screen the working-MVP smoke walk on
2026-05-07 named as the most obvious demo-tell. After this slice,
nothing on the settings page is fake — every value is fetched live
from the api.

## Acceptance criteria

- [x] New backend route `GET /api/v1/settings` (or extension of an
      existing tenant route) returns:
  - `workspace.name`, `workspace.id`,
  - `plan.name` (free / starter / growth / scale — from `subscriptions`
    table), `plan.quota_leads_per_month`,
  - `usage.leads_saved_this_period` (unique persons saved by the
    tenant since the period start),
  - `email_accounts`: array of
    `{id, email, provider, status: connected|token_expired|scope_missing, last_polled_at}`.
- [x] Frontend settings page renders all of the above. No
      hard-coded "Free" or "100 leads/mo" anywhere in
      `frontend/src/`.
      **Note:** the marketing pages
      (`/pricing`, `/(marketing)/page.tsx`, vs/* comparison pages)
      retain the strings "Free" and "100 leads/mo" — those describe
      the actual free-tier offering to anonymous visitors, not the
      user's state, and stay in place.
- [x] Per-mailbox status indicator: green dot (connected),
      amber (token expired — show "Reconnect" CTA), red (scope
      missing — show "Re-authorise" CTA leading back through the
      OAuth dance from #3).
      **Note:** the backend currently surfaces only `connected` and
      `token_expired` because `gmail_accounts` does not yet store
      the granted scope set — deriving `scope_missing` cleanly needs
      either a `scopes TEXT[]` column or an event written when
      `gmail.Sender` returns `ErrScopeMissing`. The frontend handles
      `scope_missing` correctly (red dot + "Re-authorise" CTA) for
      when that wiring lands.
- [x] Disconnect Gmail: a confirmation modal then `DELETE
      /api/v1/email-accounts/<id>`. Backend revokes the OAuth
      token (Gmail revoke endpoint), removes the row, no longer
      sends from that mailbox. Polling tick (from #4) skips
      disconnected accounts.
      **Note:** the actual wired route is
      `DELETE /api/v1/gmail/accounts/{id}` (`gmail_accounts` table
      stores the OAuth grant; `email_accounts` is the legacy SMTP
      path). The behavior — revoke at Google's endpoint before
      deleting the local row — is implemented and unit-tested.
      Poller already skips disconnected accounts because
      `ListGmailAccountsForPolling` walks the `gmail_accounts` table
      row-by-row.
- [x] Reconnect different account: from the empty / disconnected
      state, "Connect Gmail" button kicks off the OAuth flow and
      lands at the settings page with the new account row.
      "Connect Gmail" calls `GET /api/v1/gmail/auth-url` and
      `window.location.href = url` for the consent screen. The
      existing `/gmail/callback` route persists the account row;
      the next `loadSettings()` fetch picks it up.
- [x] Unit tests for the new settings handler: validation paths
      (missing tenant, wrong tenant) and happy-path JSON shape.
- [x] No regression on existing settings routes (workspace
      preferences, etc.). `go test ./...` green across the backend
      after wiring the new `/settings` route.
- [ ] Manual local verification: visit `/settings`, confirm every
      value is real; disconnect Gmail, confirm row disappears;
      reconnect via OAuth, confirm row returns with `connected`
      status.
- [ ] Once the manual local verification above is green, return to
      [#3](./03-gmail-oauth-send.md) and tick its deferred manual-
      verification criterion (single-lead campaign → message
      arrives → `email_events.gmail_message_id` populated). #3's
      send path was implementation-complete on 2026-05-14 but
      couldn't be exercised end-to-end until #8 ships the
      "Connect Gmail" UI.

## Modules touched

- `backend/internal/handler/` — new or extended `settings.go`
  handler.
- `backend/queries/tenants.sql` — extension query for usage count
  (or a new dedicated query under `backend/queries/usage.sql`).
- `backend/internal/handler/email_accounts.go` — extension for
  disconnect (Gmail revoke + row delete).
- `backend/internal/gmail/oauth.go` — revoke helper if not already
  present.
- `frontend/src/app/(app)/settings/page.tsx` — full rewrite to
  fetch + render real data.

## Test prior art

- `backend/internal/handler/tenant_leads_test.go` — handler test
  pattern for tenant-scoped routes.
- `backend/internal/handler/privacy_test.go` — handler validation
  pattern.

## Out of scope

- Plan upgrade flow — see [#10](./10-live-stripe-billing.md).
- Account deletion (full erasure) and data export — see
  [#11](./11-account-export-delete.md).
- Outlook / SMTP mailbox connect — explicitly out of scope per
  PRD.
- Per-mailbox sending preferences (custom signature, reply-to,
  per-mailbox quotas) — defer.
- Workspace member invites / multi-tenant invites — explicitly
  deferred per PRD.
