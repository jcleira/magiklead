# 08 — Account restriction / disconnect handling + reconnect

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Connect a LinkedIn account](./01-connect-linkedin-account.md), [04 — Send the connection invite (paced)](./04-send-connection-invite.md)

## What to build

When LinkedIn restricts or disconnects a connected account, the system
detects it, pauses that account's in-flight work, and prompts the user
to reconnect — so sends fail safe and visibly rather than silently.

End-to-end:
- Detection from two sources: a Unipile account-status webhook
  (`account.disconnected` / restricted), and classified send errors from
  the tick (the `restricted`/auth sentinels from #4). Either flips
  `linkedin_accounts.status` to `restricted`/`disconnected` and records
  `last_error`.
- A restricted/disconnected account's in-flight leads (`awaiting_accept`,
  `active`) are paused (not sent) until the account is healthy again.
- Settings surfaces the account as restricted with a reconnect action
  (re-run the connect flow from #1).

## Acceptance criteria

- [ ] An `account.disconnected`/restricted webhook flips
  `linkedin_accounts.status` + sets `last_error`; idempotent; assert via
  DB.
- [ ] A send returning the `restricted` sentinel flips the account to
  `restricted` and writes a `failed` event without advancing the lead;
  assert via DB.
- [ ] The tick skips accounts whose status is `restricted`/`disconnected`
  (no sends); their leads stay put; assert via DB + logs.
- [ ] Reconnecting (the #1 flow) flips the account back to `active` and
  the tick resumes it; assert via DB.
- [ ] Settings UI shows restricted status + a reconnect control
  (Playwright).

## Modules touched

- `internal/handler/unipile.go` — account-status webhook handling.
- `internal/worker/linkedin.go` — skip unhealthy accounts; flip status
  on classified send errors.
- `internal/linkedin/unipile` — status sentinels (extend #4's classified
  errors).
- `linkedin_accounts` queries; frontend settings status/reconnect.

## Test prior art

- The Clerk / gmail webhook tests — status event ingest.
- `internal/gmail/sender.go` classified errors → account-needs-reconnect
  handling.

## Out of scope

- The acceptance-rate pause
  ([07](./07-pacer-warmup-breaker-withdrawal.md)) — a different pause
  reason.
