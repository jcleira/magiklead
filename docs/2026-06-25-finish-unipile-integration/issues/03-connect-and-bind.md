# 03 — Connect-and-bind from real payload

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Webhook auth + real-event routing](./02-webhook-auth-event-routing.md)

**Confirmed from spike (fill after #1):** auto-bind works with zero SQL
= **NO — broken** (spike finding A: the connect webhook carries no tenant
metadata and the hosted-auth request sets no `notify_url`; adding
`notify_url` + binding on decodable metadata is THIS slice's job);
metadata echo timing within the validity window = **not measurable until
`notify_url` exists** — measure during this slice's live connect and
extend the 15-min window if needed.

## What to build

When a real inbound payload carries our **decodable signed metadata**
(the tenant+user token minted into the hosted-auth link), treat it as
**connect-and-bind**: upsert the `linkedin_account` and bind it to the
tenant recovered from the metadata — **regardless of which Unipile
source the message arrives on** — with **zero manual SQL**. A payload
**without** metadata that reports a status is NOT a connect (that's
#4). The operator connects a LinkedIn account in the app and it is
linked to their workspace automatically.

## Acceptance criteria

- [x] POSTing the captured `account.connected` body with the real
  `Unipile-Auth` header upserts a `linkedin_account` **bound to the
  tenant** recovered from the signed metadata — no manual SQL.
- [x] Any inbound payload carrying decodable signed metadata is treated
  as connect-and-bind regardless of source; a payload without metadata
  that reports a status is **not** treated as connect.
- [x] The signed-metadata validity window admits the real echo timing
  (extend beyond the current 15 min **only if** #1 showed it necessary).
- [x] After binding, the account surfaces a clear connected/active
  status (US 2).
- [x] A handler **integration test** drives the captured connect body
  end-to-end (in-process HTTP server + real DB) and asserts the bound
  account row + its tenant.

## Modules touched

- Webhook handler (`backend/internal/handler/unipile.go` —
  `handleAccountConnected`, `decodeUnipileState`, the metadata TTL
  constant; `encodeUnipileState` in `AuthURL` mints the token).
- `UpsertLinkedInAccount` (`backend/queries/linkedin_accounts.sql` +
  generated repository) — idempotent on `unipile_account_id`, tenant
  pinned on first insert.

## Test prior art

- `backend/internal/handler/unipile_test.go` — AuthURL metadata signing,
  free-tier gate.
- The connect path of
  `backend/internal/handler/unipile*integration_test.go` — **rebuild on
  the real connect fixture** using an in-process HTTP server.

## Out of scope

- Status transitions after connect — **#4**.
- Reply handling — **#8**.
- The free-tier gate (1 connected account per tenant) already exists
  from the prior effort; this slice does not change it.
