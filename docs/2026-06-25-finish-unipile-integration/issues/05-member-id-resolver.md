# 05 — Member-id resolver

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Validation spike](./01-validation-spike.md)

**Confirmed from spike (fill after #1):** resolve-response shape /
endpoint = `GET /api/v1/users/<public_identifier>?account_id=<acc>` →
bare `UserProfile` object; the member id is **`provider_id`** (`ACoAA…`).
Beware: this response's `member_urn` is numeric — do not use it. Frozen
in [spike-captures §6](../spike-captures.md) (captured 2026-07-06).

## What to build

A **read-through cache** that turns a prospect's LinkedIn profile URL
into the Unipile **member id** needed to send anything, and remembers it
**permanently** on the person. The Unipile client gains a **resolve**
operation: (profile URL + account) → member id, or a **classified
error**. Resolution is **lazy** — it runs the first time we contact a
person: check the person's saved member id, call Unipile **only on a
miss**, then persist the result. Every later step and every other
campaign reuses the cached id.

Failures are classified so the queue keeps moving:
- **Transient** (rate-limit, upstream 5xx, timeout) → leave the lead
  queued, write **no** failure event, do **not** consume pacing
  allowance; retry next tick.
- **Permanent** (not found, deleted, private/unresolvable) → write a
  `failed` event and move the lead to a terminal `failed` status.

This slice builds the resolver as a unit; #6 wires it into the actual
send path.

## Acceptance criteria

- [x] The Unipile client exposes a `resolve` op: (profile URL +
  account) → member id, or a classified error (transient | permanent),
  built/tested against the **real resolve-response shape** from #1.
- [x] The member id is cached on the person as a `person_identifiers`
  row of type **`linkedin_member_id`** (no new per-lead column); the
  existing `linkedin_url` identifier is the resolution input.
- [x] **Read-through:** a cache hit returns without calling Unipile; a
  miss calls Unipile **once** and persists the result (persist-once,
  idempotent — the same person is never looked up twice, even across
  campaigns).
- [x] **Transient** failure → lead left queued, **no** failure event,
  pacing allowance **not** consumed.
- [x] **Permanent** failure → a `failed` event written to
  `linkedin_events` and the `campaign_lead` moved to a terminal
  `failed` status (not retried).
- [x] `failed` is added as a new value in the free-text
  `campaign_leads.status` column — **no migration required**.
- [x] Resolver **unit tests** drive a fake Unipile client + the store:
  cache hit vs miss, persist-once, and transient-vs-permanent handling
  (queued-and-retried vs failed-and-terminal).

## Modules touched

- Unipile client (`backend/internal/linkedin/unipile/unipile.go` — new
  `Resolve` op + error classification alongside `classifySendStatus`).
- `person_identifiers` read/write (`CreatePersonIdentifier` /
  `FindPersonByIdentifier` in `backend/queries/ingest.sql` + generated
  repository) with the new `linkedin_member_id` type.
- `campaign_leads` terminal `failed` status + a `failed`
  `linkedin_events` row (`backend/queries/campaign_leads.sql`,
  `backend/queries/linkedin_events.sql`).
- A resolver helper in the worker package
  (`backend/internal/worker/`).

## Test prior art

- Existing fake-client / repository tests under
  `backend/internal/linkedin/unipile/*_test.go` and
  `backend/internal/worker/*_test.go`.

## Out of scope

- Wiring the resolver into the real send path — **#6**.
- The list-connections op and acceptance matching — **#7**.
