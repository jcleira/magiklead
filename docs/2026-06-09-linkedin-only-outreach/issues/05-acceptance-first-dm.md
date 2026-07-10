# 05 — Acceptance → first DM

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [04 — Send the connection invite (paced)](./04-send-connection-invite.md)

## What to build

When a prospect accepts the connection request, the system notices
(Unipile webhook) and sends the first DM on the next tick. This is the
event-gated transition that turns a parked invite into a live
conversation.

End-to-end:
- `POST /webhooks/unipile` (from #1) gains handling for the
  `invitation-accepted` event: map it to the `campaign_lead` (by
  `linkedin_invitation_id`/account), set `accepted_at`, flip `status`
  `awaiting_accept` → `active`, `next_send_at = NOW()`,
  `current_step = 1`, and write a `linkedin_events` `accepted` row.
- The existing LinkedIn tick (#4) then picks the lead up (it's `active`,
  due) and sends DM step 1 via Unipile send-message into the chat;
  writes `dm_sent` with the chat id; advances `current_step` /
  `next_send_at` to the next step's delay (reuse the email advance
  logic, with jitter).

## Acceptance criteria

- [x] An `invitation-accepted` webhook for a known
  `linkedin_invitation_id` flips the lead to `active`, sets
  `accepted_at` + `next_send_at=NOW()` + `current_step=1`, writes an
  `accepted` event; idempotent on replay. Assert via DB.
- [x] On the next tick the lead gets DM step 1 sent via Unipile
  send-message (assert payload: account_id, chat/recipient, rendered
  body) → `dm_sent` event with `unipile_chat_id` → `current_step` /
  `next_send_at` advanced to step 2's delay. Assert via DB + stub.
- [x] An accept webhook for an unknown invitation id is a safe no-op 200
  (idempotent), logged.
- [x] Acceptance updates the account's acceptance-rate signal used later
  by [07](./07-pacer-warmup-breaker-withdrawal.md) (an `accepted` event
  exists to count).

## Modules touched

- `internal/handler/unipile.go` — invitation-accepted handling + lead
  mapping.
- `internal/worker/linkedin.go` — DM send for `active` leads at step ≥1
  (reuse the advance logic from `internal/worker/sender.go`).
- `internal/linkedin/unipile` — send-message (DM).
- `campaign_leads`, `linkedin_events` queries.

## Test prior art

- `internal/worker/poller_test.go` — webhook/event → campaign_lead state
  transition + event write.
- `internal/worker/sender_test.go` — step advance + jitter.

## Out of scope

- Follow-ups beyond step 1, reply detection/halt
  ([06](./06-followups-reply-halt.md)).
- Withdrawing never-accepted invites
  ([07](./07-pacer-warmup-breaker-withdrawal.md)).
