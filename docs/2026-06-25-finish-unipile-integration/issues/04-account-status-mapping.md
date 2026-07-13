# 04 — Account status mapping

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Webhook auth + real-event routing](./02-webhook-auth-event-routing.md)

**Confirmed from spike (fill after #1):** real status vocabulary =
`AccountStatus.message`; only **`CREATION_SUCCESS`** captured. Healthy
restarts emit no webhook (spike finding E) — other values are not
capturable on demand; treat unknown values conservatively and capture
reconnect/restriction transitions opportunistically (#3's live connect
is the next emitter). Note: `sources[].status` (`OK`) on the account
object is a separate API-side vocabulary, not this webhook's.

## What to build

Map Unipile's **real** status vocabulary (verified against the captured
payloads) to the existing `active` / `restricted` / `disconnected`
account states. **Unknown statuses are ignored** rather than clobbering
the current value. A restriction/disconnect stops sending and records
the provider reason; recovery re-activates so campaigns resume.

## Acceptance criteria

- [x] Real status strings map to `active` / `restricted` /
  `disconnected` per the vocabulary confirmed in #1.
- [x] An unknown/unrecognized status leaves the current account status
  **unchanged** (no clobber).
- [x] A restriction/disconnect status records the provider reason (e.g.
  `last_error`) and the worker **stops sending** for that account
  (restricted/disconnected accounts are already excluded from the send
  path — verify the gate still holds).
- [x] A recovery status **re-activates** the account so sending resumes
  (US 23).
- [x] A status-update payload (no signed metadata) updates an
  **existing** account; it does NOT create one (that's connect, #3).
- [x] Handler **integration tests** drive the captured status payloads
  and assert the resulting account status + recorded reason.

## Modules touched

- Webhook handler (`backend/internal/handler/unipile.go` —
  `handleAccountStatus` + the status-mapping function).
- `SetLinkedInAccountStatusByUnipileID`
  (`backend/queries/linkedin_accounts.sql` + generated repository).
- The send-path status gate in the worker
  (`backend/internal/worker/linkedin.go`; `GetDueLinkedInDMLeads` /
  invite selection filter on account status) — verify, don't rewrite.

## Test prior art

- `backend/internal/handler/unipile*status*integration_test.go` —
  **rebuild on the real status fixtures**.

## Out of scope

- Connect-and-bind — **#3**.
- The reconnect UI — already built from the prior effort.
- Pacing / warmup / breaker thresholds — unchanged.
