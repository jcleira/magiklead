# 07 — Warmup ramp + acceptance breaker + stale-invite withdrawal

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [04 — Send the connection invite (paced)](./04-send-connection-invite.md)

## What to build

The full Standard pacer safety policy on top of #4's basic caps: a
multi-week warmup ramp, an acceptance-rate circuit breaker that pauses a
struggling account, and withdrawal of invitations that were never
accepted.

End-to-end:
- Warmup ramp: a freshly connected account starts low and ramps —
  ~8 invites/day in week 1 to ~20/day by week 4 — driven off
  `linkedin_accounts.warmup_started_at`. The pacer's allowance respects
  the ramp in addition to the 100/week ceiling.
- Acceptance circuit breaker: if an account's trailing-7-day acceptance
  rate falls below 20%, the pacer returns 0 (account paused) and the
  account is flagged so the UI can surface it; it resumes when the rate
  recovers.
- Stale-invite withdrawal: invitations still `awaiting_accept` past
  21 days are withdrawn via Unipile and the lead closed `not_accepted`
  (with a `withdrawn` event).

## Acceptance criteria

- [ ] Pacer unit tests across the warmup curve: day-of-warmup →
  allowed/day matches the ramp (≈8 → 20 over wk1 → wk4), capped by
  100/week; injected clock. Pure logic.
- [ ] Acceptance breaker: with counters showing <20% trailing-7-day
  acceptance, the pacer returns 0 and the account is marked paused; ≥20%
  resumes. Unit-tested.
- [ ] Withdrawal: a lead `awaiting_accept` with an invite older than
  21 days is withdrawn (Unipile cancel-invitation called — assert
  against the stub), lead → `not_accepted`, a `withdrawn` event written.
  Assert via DB.
- [ ] The acceptance-rate signal is computed from `linkedin_events`
  (`invite_sent` vs `accepted`) over 7 days.

## Modules touched

- `internal/linkedin/pacer` — extend with the warmup ramp + acceptance
  breaker (the deep module's full policy).
- `internal/worker/linkedin.go` — the withdrawal sweep for stale
  `awaiting_accept` leads.
- `internal/linkedin/unipile` — cancel-invitation.
- `linkedin_accounts` (paused state), `linkedin_events` (acceptance-rate
  query).

## Test prior art

- The pacer's #4 tests are the direct base to extend; table-test style
  as in `internal/leads/pdl` pure-logic suites.

## Out of scope

- Restriction/disconnect from Unipile errors
  ([08](./08-account-restriction-reconnect.md)) — distinct from the
  acceptance-rate pause here.
