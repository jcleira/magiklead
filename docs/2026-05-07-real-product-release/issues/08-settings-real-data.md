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

- [ ] New backend route `GET /api/v1/settings` (or extension of an
      existing tenant route) returns:
  - `workspace.name`, `workspace.id`,
  - `plan.name` (free / starter / growth / scale — from `subscriptions`
    table), `plan.quota_leads_per_month`,
  - `usage.leads_saved_this_period` (unique persons saved by the
    tenant since the period start),
  - `email_accounts`: array of
    `{id, email, provider, status: connected|token_expired|scope_missing, last_polled_at}`.
- [ ] Frontend settings page renders all of the above. No
      hard-coded "Free" or "100 leads/mo" anywhere in
      `frontend/src/`.
- [ ] Per-mailbox status indicator: green dot (connected),
      amber (token expired — show "Reconnect" CTA), red (scope
      missing — show "Re-authorise" CTA leading back through the
      OAuth dance from #3).
- [ ] Disconnect Gmail: a confirmation modal then `DELETE
      /api/v1/email-accounts/<id>`. Backend revokes the OAuth
      token (Gmail revoke endpoint), removes the row, no longer
      sends from that mailbox. Polling tick (from #4) skips
      disconnected accounts.
- [ ] Reconnect different account: from the empty / disconnected
      state, "Connect Gmail" button kicks off the OAuth flow and
      lands at the settings page with the new account row.
- [ ] Unit tests for the new settings handler: validation paths
      (missing tenant, wrong tenant) and happy-path JSON shape.
- [ ] No regression on existing settings routes (workspace
      preferences, etc.).
- [ ] Manual local verification: visit `/settings`, confirm every
      value is real; disconnect Gmail, confirm row disappears;
      reconnect via OAuth, confirm row returns with `connected`
      status.

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
