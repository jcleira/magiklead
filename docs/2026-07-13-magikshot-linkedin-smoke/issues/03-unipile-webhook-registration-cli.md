# 03 — Unipile webhook registration CLI: list / register / prune

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately. (Its live run against
the tunnel hostname happens in #06; development and tests need no
tunnel.)

## What to build

An idempotent, checked-in CLI that manages the app's webhook
registrations at Unipile, so recovering from a dead registration is
one command instead of dashboard archaeology (PRD user stories 12,
13). This is greenfield: nothing in the codebase calls Unipile's
`/api/v1/webhooks` endpoints today — the only prior art is the
manual curl recipe in
`docs/2026-06-25-finish-unipile-integration/spike-captures.md`
("Webhook registration" section), which registered two sources; the
smoke needs **three**:

- `account_status` — account connected / status changes
- the relations source (invitation acceptances; confirm Unipile's
  exact source name during implementation — the spike never
  registered it; the inbound handler already routes
  `invitation_id`-marked payloads)
- `messaging` with `"events": ["message_received"]`

Each registration: `request_url = <base>/api/v1/webhooks/unipile`,
`headers: [{"key": "Unipile-Auth", "value": $UNIPILE_WEBHOOK_SECRET}]`
(the endpoint's only auth — a constant-time static-header compare in
`Module.VerifyAuthToken`, `backend/internal/linkedin/unipile/unipile.go`;
Unipile has no body signing), and a recognizable `name` marker (e.g.
`magiklead-…`) so prune targets only our registrations.

Shape:

- **Client additions** in `backend/internal/linkedin/unipile`:
  `ListWebhooks` / `CreateWebhook` / `DeleteWebhook` wrapping
  `GET|POST /api/v1/webhooks` and `DELETE /api/v1/webhooks/{id}`,
  going through the existing `do()` (which injects `X-API-KEY`
  against `UNIPILE_DSN`). The `Module`'s injectable `Doer` is the
  test seam.
- **CLI** at `backend/cmd/unipile-webhooks/main.go`, following the
  house convention (`godotenv.Load()`, stdlib `flag`, header comment
  documenting the exec path like `cmd/seed`): modes `list`,
  `register --base-url <origin>`, `prune`, plus a global `--dry-run`.
  Register is idempotent: list first, create only what's missing
  (match on source + request_url), report created/kept. Prune
  deletes only registrations carrying our name marker (optionally
  filtered to stale base URLs), reports deletions. Dry-run prints
  the exact plan and performs zero mutating calls.
- **Doc drift fix (found during exploration):** two stale comments
  claim the webhook is authenticated by an HMAC body signature —
  the route comment above `r.Post("/webhooks/unipile", …)` in
  `backend/cmd/api/main.go` and the CLAUDE.md sentence about
  "crafted payloads … with the right signature". Correct both to
  the static `Unipile-Auth` header. No behavior change.

## Acceptance criteria

- [x] `list` prints current registrations (id, source, request_url,
      name) from `GET /api/v1/webhooks`.
- [x] `register --base-url <origin>` run twice against a stub:
      first run creates the three sources (correct request_url,
      auth header, events for messaging), second run creates
      nothing and reports kept/skipped.
- [x] `prune` deletes only registrations bearing the CLI's name
      marker; foreign registrations are untouched (asserted in the
      stub test).
- [x] `--dry-run` prints the plan and issues zero POST/DELETE calls
      (test asserts no mutating requests hit the stub).
- [x] Tests: dry-run plan assertion + happy-path register +
      idempotent re-run, all against an `httptest` stub via the
      Module's `Doer` seam (PRD Testing Decisions).
- [x] The two stale HMAC comments are corrected.
- [x] Header comment documents the in-pod exec path
      (`devpods exec api go run ./cmd/unipile-webhooks …`).

## Modules touched

- `backend/cmd/unipile-webhooks/` (new).
- `backend/internal/linkedin/unipile/` (webhook CRUD client methods
  + tests).
- `backend/cmd/api/main.go` (comment only), `CLAUDE.md` (one
  sentence).

## Test prior art

- Unipile client unit tests with injected `Doer`/stubbed HTTP:
  `backend/internal/linkedin/unipile/*_test.go`.
- Flag-driven `cmd/` CLIs: `backend/cmd/ingest`,
  `backend/cmd/verify-emails`.

## Out of scope

- Running it against the real tunnel hostname — #06's ceremony prep.
- The inbound webhook handler — already correct (static-header
  verify); no endpoint changes.
- Tunnel/DNS (#01), base-URL override (#02).
