# 06 — Follow-up DMs, reply-halt, reconcile poll

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [05 — Acceptance → first DM](./05-acceptance-first-dm.md)

## What to build

DM follow-ups fire on their delays until the prospect replies; a reply
halts the sequence immediately and suppresses the person. A
low-frequency reconciliation poll backstops any webhook that didn't
arrive, so no lead is stranded.

End-to-end:
- Follow-up DM steps (2..n) advance on `delay_days` timers exactly like
  the email engine; this slice exercises the multi-step path to
  exhaustion (`exhausted` after the last step).
- `POST /webhooks/unipile` inbound-message event → map to the
  `campaign_lead` by `linkedin_chat_id` → write a `replied` event, flip
  the lead `replied`, and suppress the person (person-keyed
  suppression). No further DMs ever go to a replied lead — the
  zero-DMs-to-replied invariant, enforced through the suppression gate
  in the tick.
- Suppression extended: `unsubscribes.person_id` (nullable) + a
  person-keyed `IsSuppressed`/record path; `RecordReply` gains a person
  path (today it keys on email). Only `reply`/`manual` reasons apply on
  LinkedIn (no bounces).
- A reconciliation poll (~30–60 min) lists recent invitation-status +
  chat activity per account via Unipile and applies any accept/reply a
  webhook missed (idempotent with the webhook path).

## Acceptance criteria

- [x] Migration adds `unsubscribes.person_id` (nullable) + the
  unique-index update; a person-keyed suppression query.
- [x] Multi-step: a lead with the first DM sent advances through
  follow-ups on their delays and ends `exhausted` after the last step;
  assert via DB.
- [x] An inbound-message webhook for a known `linkedin_chat_id` writes a
  `replied` event, flips the lead to `replied`, and inserts a
  person-keyed suppression row; assert via DB.
- [x] The send tick refuses to DM a replied/suppressed lead (suppression
  gate) — the zero-DMs-to-replied invariant holds even if a step was
  due; assert via DB + a `skipped` event.
- [x] Reconcile poll: with the webhook suppressed in a test, a poll tick
  detects an accept and a reply from the Unipile stub and applies the
  same transitions idempotently (no double events). Assert via DB.
- [x] `IsSuppressed` still works for the email path unchanged
  (regression).

## Modules touched

- `internal/suppression` — person-keyed `IsSuppressed` + `RecordReply`
  (extend the email-keyed module).
- `internal/handler/unipile.go` — inbound-message event → reply.
- `internal/worker/linkedin.go` — suppression gate before each DM;
  follow-up advance to exhaustion; the reconcile-poll loop (mirror
  `internal/worker/poller.go`).
- `internal/linkedin/unipile` — list-chats / list-invitation-status for
  reconcile.
- `unsubscribes` schema + suppression queries.

## Test prior art

- `internal/suppression/suppression_test.go` — extend for person-keyed
  suppression + reply-halt with no email.
- `internal/worker/poller_test.go` — reply → replied +
  zero-send-to-replied invariant; reconcile/idempotency.

## Out of scope

- Pacer warmup/breaker/withdrawal
  ([07](./07-pacer-warmup-breaker-withdrawal.md)). Metrics
  ([09](./09-metrics-capacity-dashboard.md)).
