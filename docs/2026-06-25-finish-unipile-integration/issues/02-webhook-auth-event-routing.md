# 02 — Webhook auth + real-event routing

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Validation spike](./01-validation-spike.md)

**Confirmed from spike (fill after #1):** real event marker field +
values = `account_status` payloads wrap under a top-level `AccountStatus`
key (status in `AccountStatus.message`); `messaging` payloads carry a
top-level `"event"` marker (`"message_received"`) with a **flat** body —
real raw bytes captured and frozen in
[spike-captures §9](../spike-captures.md) (raw stash:
`~/.config/devpods/magiklead/captures/`). **Type gotcha (finding F):
webhook `is_sender` is a bool** (REST `/messages` uses int `0/1`) — the
first live delivery 400'd the current int-typed struct; type the two
surfaces separately. Keys are conditional per event (`mentions`/
`reaction`/`reaction_sender`/`read_by` declared but absent in §9).
Webhook auth header name/scheme = static
`Unipile-Auth: <UNIPILE_WEBHOOK_SECRET>` header, constant-time compare —
verified live 2026-07-01 and re-verified on the real §9 delivery (auth
passed, parse failed).

## What to build

Rebuild the Unipile webhook so it authenticates exactly the way **real
Unipile** authenticates, and routes inbound payloads on Unipile's
**real event marker** in the body (replacing today's object-presence
guesswork). Feed the frozen fixtures from #1 into the parser and prove
it classifies every real payload type and recovers the right ids +
reply direction. This slice establishes the parsing/auth skeleton the
account/reply behaviors (#3, #4, #8) hang on — it parses and
authenticates but does not yet act on the events.

Authentication is **only** the fixed `Unipile-Auth` header, compared in
constant time to the configured webhook secret. The legacy
fake-signature (HMAC body) path — which only ever existed to satisfy
self-authored tests — is removed. The temporary capture log is removed
now that the fixtures are frozen.

## Acceptance criteria

- [x] The webhook returns **503** when the secret is unset, **401** on
  a missing/incorrect `Unipile-Auth` header, **200** otherwise
  (including for event types we don't act on).
- [x] The legacy fake-signature path (`X-Unipile-Signature` / HMAC body
  verification) is **removed** from both client and handler.
- [x] `ParseWebhook` routes on the **real event marker** confirmed in #1
  — not on object-presence guesswork — and classifies connect vs status
  vs message, recovering the account id, metadata token, and
  invitation/chat/message ids + message direction.
- [x] Parser **unit tests** feed the frozen raw fixture bytes and assert
  the typed result for every type, including reply-direction in **both**
  directions (a self message vs a prospect reply).
- [x] The **imagined-payload tests** are deleted (the hardcoded /
  guessed-shape tests that prove a contract that doesn't exist).
- [x] The **temporary capture log** (`UNIPILE-CAPTURE`) is removed from
  the webhook handler.

## Modules touched

- Unipile client parsing (`backend/internal/linkedin/unipile/unipile.go`
  — `ParseWebhook`, `VerifyAuthToken`; remove `VerifySignature`).
- Webhook handler (`backend/internal/handler/unipile.go` — `Webhook()`
  auth + routing; remove the capture-log block).
- `UNIPILE_WEBHOOK_SECRET` (the configured webhook secret).

## Test prior art

- `backend/internal/linkedin/unipile/*_test.go` — client unit tests with
  a fake HTTP doer; **rebuild on the real fixtures**.
- `backend/internal/handler/unipile_test.go` — handler unit tests
  covering the auth paths.

## Out of scope

- Acting on the parsed events — connect-and-bind is **#3**, status
  mapping is **#4**, reply handling is **#8**.
- Member-id resolution (**#5**).
