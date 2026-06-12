# 01 — Connect a LinkedIn account

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A tenant user connects their own LinkedIn account through Unipile's
hosted-auth wizard — the LinkedIn analogue of the existing Gmail OAuth
connect flow. They click "Connect LinkedIn" in settings, complete
Unipile's hosted login, and the account appears as `active` in
settings. The free tier allows exactly one connected account.

End-to-end:
- Backend mints a Unipile hosted-auth link (`GET /linkedin/auth-url`),
  embedding a signed `pkg/jwt` token (tenant_id + user_id) as the
  Unipile account metadata. Because Unipile owns the redirect (unlike
  the Gmail callback we host), the tenant binding rides in metadata,
  not a self-hosted callback.
- Unipile fires the `account.connected` (and account-status) webhook to
  a new public `POST /webhooks/unipile`; the handler verifies the
  signature, recovers tenant/user from the signed metadata, and upserts
  a `linkedin_accounts` row.
- Settings lists connected LinkedIn accounts with status, behind a
  risk-consent step; a disconnect (DELETE) removes the account.
- Connecting a second account on the free tier is refused.

## Acceptance criteria

- [ ] Migration creates `linkedin_accounts` (id, tenant_id,
  unipile_account_id, status, warmup_started_at, weekly_invite_count,
  weekly_window_started_at, daily_invite_count, daily_reset_at,
  acceptance_rate, last_error, created_at). Status ∈
  connecting|active|warming|restricted|disconnected.
- [ ] `GET /api/v1/linkedin/auth-url` (Clerk-auth + tenant) returns a
  Unipile hosted-auth URL; assert via the stubbed Unipile `Doer` that a
  signed `pkg/jwt` token (tenant_id+user_id, short TTL) is sent and
  `jwt.Decode` round-trips it.
- [ ] `POST /api/v1/webhooks/unipile` is public, verifies the
  `UNIPILE_WEBHOOK_SECRET` signature (503 if unconfigured, 401 on bad
  sig — mirror the Clerk handler), and is idempotent (replaying the same
  `account.connected` upserts one row).
- [ ] A valid `account.connected` webhook inserts a `linkedin_accounts`
  row with status `active` for the bound tenant; assert via DB.
- [ ] `GET /api/v1/linkedin/accounts` lists the tenant's accounts with
  status; `DELETE /api/v1/linkedin/accounts/{id}` removes one (calls
  Unipile disconnect via the stub).
- [ ] On the free plan, a second connect attempt returns a 4xx capacity
  error and writes no second row; assert via DB.
- [ ] Settings UI: "Connect LinkedIn" gated behind a risk-consent
  checkbox; connected accounts render with status; a disconnect control.
  Playwright covers the happy path against a stubbed backend.
- [ ] `UNIPILE_API_KEY` absent → routes degrade like PDL (feature
  disabled, no fatal). Documented in CLAUDE.md devpod secrets.

## Modules touched

- New `internal/linkedin/unipile` deep module — Unipile API behind a
  `Doer` HTTP-client seam (mirror `internal/leads/pdl`): hosted-auth
  link, disconnect, parse webhook payloads into typed events.
- New `internal/handler/unipile.go` — auth-url + accounts handlers and
  the public webhook handler (mirror `internal/handler/gmail.go` +
  `internal/handler/clerk.go`).
- `pkg/jwt` — reuse `Encode`/`Decode` for the connect-state token.
- `linkedin_accounts` table + repository queries.
- Config in `cmd/api/main.go`: `UNIPILE_API_KEY`, `UNIPILE_DSN`,
  `UNIPILE_WEBHOOK_SECRET` (required-when-key-set idiom).
- Frontend settings connect surface (clone the Gmail-connect component).

## Test prior art

- `internal/leads/pdl/*_test.go` — httptest-stub `Doer` table tests for
  an external-API seam.
- The Clerk webhook handler test — signature verify (503/401) +
  idempotent ingest.
- `internal/handler/gmail*` — OAuth connect handler + signed-state
  round-trip; `pkg/jwt` tests for Encode/Decode.
- Frontend Playwright e2e suite — the Gmail-connect flow as template.

## Out of scope

- Any sending (invites/DMs) — see [04](./04-send-connection-invite.md).
- Multi-account beyond the 1-account free gate — deferred (PRD Out of
  Scope).
- Restriction/disconnect *detection from send errors* — see
  [08](./08-account-restriction-reconnect.md); this slice only stores
  status as set by the connect/status webhook.

## Handoff

When the ACs pass, BEFORE flipping this slice's row in `issues.md`, post
to the user verbatim:

- **URL / artefact to visit**: the devpod settings page
  (`http://<devpod>.magiklead.localhost/settings`), with the Unipile
  webhook reachable via a cloudflared/ngrok tunnel — the same tunnel
  trick used for the Gmail connect flow (see CLAUDE.md → "Connecting
  Gmail in the devpod" runbook).
- **Action required**: connect a REAL LinkedIn account through the
  Unipile hosted flow (human credentials — cannot be stub-automated)
  and confirm it lands `active`.
- **Where to record the decision**:
  - In `issues.md` (one-line note next to this row: the connected
    `unipile_account_id`), AND
  - In [04-send-connection-invite.md](./04-send-connection-invite.md)
    under `Connected account id:` — slice #4's live verification uses it.

Slice #4's live-path verification MUST NOT start until that field is
filled. (Slice #4's automated tests run without it, against the stub.)
