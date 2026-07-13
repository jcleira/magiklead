# 04 — Send the connection invite (paced)

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Connect a LinkedIn account](./01-connect-linkedin-account.md), [03 — Author a LinkedIn campaign](./03-author-linkedin-campaign.md)

**Connected account id**: _(fill from slice #1's Handoff before live
verification; automated tests run against the Unipile stub without it)_

## What to build

The worker turns queued LinkedIn `campaign_leads` into sent connection
requests, paced under safe limits, then parks each lead waiting for
acceptance. This is step 0 of the sequence.

End-to-end:
- New `linkedin/pacer` deep module — basic capacity policy for this
  slice: the weekly invite ceiling (Standard = 100/week) + a per-day
  sub-cap, reading the `linkedin_accounts` rolling counters + a clock;
  returns how many invites the account may send now. (Warmup ramp,
  acceptance breaker, and withdrawal land in
  [07](./07-pacer-warmup-breaker-withdrawal.md).)
- New worker LinkedIn tick (goroutine like `StartSendLoop`, ~60s):
  selects due `channel='linkedin'` leads (`status='queued'`/`active` at
  step 0, `next_send_at <= NOW()`), asks the pacer for allowance, sends
  the invite via `linkedin/unipile` with the rendered step-0 note,
  writes a `linkedin_events` `invite_sent` row, increments the account
  counters, and moves the lead to `awaiting_accept` with `next_send_at`
  cleared.
- A pacer-exhausted account simply sends fewer this tick; nothing
  over-sends.

## Acceptance criteria

- [x] Migrations: `linkedin_events` (campaign_lead_id, event_type, step,
  metadata, unipile_message_id, unipile_chat_id, created_at; indices
  like `email_events`); `campaign_leads` adds `linkedin_account_id`,
  `linkedin_invitation_id`, `linkedin_chat_id`, `accepted_at`; `status`
  enum gains `awaiting_accept`, `not_accepted`.
- [x] `linkedin/pacer` unit tests: given counters + an injected clock,
  returns the right invite allowance; never exceeds 100/week or the
  daily sub-cap. Pure logic — no DB/network.
- [x] Worker tick: a queued LinkedIn lead → Unipile send-invite called
  (assert the payload against the stub: account_id, recipient, rendered
  note) → `invite_sent` event → lead `awaiting_accept`, `next_send_at`
  NULL, `linkedin_account_id` + `linkedin_invitation_id` set → account
  `weekly_invite_count` incremented. Assert via DB + stub.
- [x] Pacer budget 0 → the tick sends nothing and leaves the lead
  queued; assert via DB + logs.
- [x] A Unipile send error is classified (rate-limit/auth/restricted
  sentinels, mirror `gmail.Sender`) and written as a `failed`
  `linkedin_event`; the lead is not advanced.
- [x] With one connected account, invites only come from that account
  (the assigned `linkedin_account_id`).

## Modules touched

- New `internal/linkedin/pacer` — capacity policy (pure-logic deep
  module).
- New `internal/worker/linkedin.go` — the LinkedIn send tick (mirror
  `internal/worker/sender.go`).
- `internal/linkedin/unipile` — send-invitation (extend the #1 seam)
  with classified errors (mirror `internal/gmail/sender.go`).
- `linkedin_events`, `campaign_leads` (new columns), `linkedin_accounts`
  (counter updates) + queries.

## Test prior art

- `internal/worker/sender_test.go` — due-lead selection, send, event
  write, state advance against a stubbed sender.
- `internal/gmail/sender.go` — classified-error pattern via httptest
  stub.
- The pacer has no existing twin — build the strongest unit suite here
  (injected clock + counters, table-driven).

## Out of scope

- Detecting acceptance / sending DMs
  ([05](./05-acceptance-first-dm.md)).
- Warmup ramp, acceptance-rate breaker, stale-invite withdrawal
  ([07](./07-pacer-warmup-breaker-withdrawal.md)) — this slice ships
  only weekly + daily caps.
- Restriction state transitions on repeated errors
  ([08](./08-account-restriction-reconnect.md)) — here a send error just
  writes a `failed` event.
