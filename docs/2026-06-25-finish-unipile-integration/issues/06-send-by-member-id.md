# 06 — Invite + DM addressed by member id

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [05 — Member-id resolver](./05-member-id-resolver.md)

## What to build

Wire the resolver (#5) into the worker send path so invites and messages
reach the **right person**. The invite step resolves the member id
(lazy, first contact) and addresses the Unipile invite **by member id**
rather than profile URL; the DM step **reuses the already-cached id**
with no extra lookup. The resolve runs **before** the pacing gate, so a
transient failure leaves the lead queued without consuming allowance.

## Acceptance criteria

- [x] The Unipile client's invite + message sends take a **resolved
  member id** rather than a profile URL.
- [x] At invite-send the worker resolves the member id (via #5) and
  addresses the invite by it; on success the id is persisted on the
  person.
- [x] The DM step **reuses the cached member id** — no second resolve,
  no extra Unipile lookup mid-sequence.
- [x] Resolve runs **before** the pacing gate: a **transient** resolve
  failure leaves the lead queued and does **not** consume pacing
  allowance; a **permanent** failure fails the lead terminally (per #5)
  without sending.
- [x] Invites still go within the existing daily/weekly caps + warmup
  ramp (pacing unchanged).
- [x] Follow-ups continue to fire on schedule, reusing the cached id
  (US 15).
- [x] A worker **integration test**: an invite resolves then sends by
  member id; a DM step reuses the cached id without resolving again.

## Modules touched

- Worker send loops (`backend/internal/worker/linkedin.go` —
  `processLinkedInQueue` invite path, `processLinkedInDMQueue` DM path).
- Unipile client `SendInvitation` / `SendMessage`
  (`backend/internal/linkedin/unipile/unipile.go`) — address by member
  id, not URL.
- The pacer gate (`backend/internal/linkedin/pacer/pacer.go`) —
  unchanged; just ordered **after** the resolve so transient resolve
  failures don't burn allowance.

## Test prior art

- `backend/internal/worker/*_test.go` — send-loop tests.
- `backend/internal/linkedin/unipile/*_test.go` — send request-shape
  tests (assert the member id is what's sent).

## Out of scope

- The resolver internals — **#5**.
- Acceptance detection — **#7**.
- Pacing / warmup / breaker threshold changes — unchanged.
