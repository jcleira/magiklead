# 22 — Uptime monitoring

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#16 — Hetzner VPS + api online](./16-hetzner-vps-and-api.md)

## What to build

Wire an external uptime monitor (Better Stack, UptimeRobot, or
operator's choice with a free tier) to ping
`https://api.magiklead.com/health` AND `https://magiklead.com`
every 60 seconds and notify the operator on three consecutive
failures. After this slice, the operator gets paged within ~3
minutes if either api or frontend goes down.

The PRD's exit criterion #5 ("uptime monitor active") plus
severity-policy P1 ("site unreachable") both depend on this slice.

## Acceptance criteria

- [ ] Monitor account exists on a free-tier-acceptable plan
      (UptimeRobot's free tier covers up to 50 monitors; Better
      Stack's free tier covers a handful).
- [ ] Two monitors configured:
  - api: target `https://api.magiklead.com/health`, expected
    response `200 OK` with body containing `ok`, interval 60s,
    failure threshold 3 consecutive,
  - frontend: target `https://magiklead.com`, expected `200`
    with a small expected substring (e.g. "magiklead"), interval
    60s, threshold 3.
- [ ] Notification channel configured to reach the operator
      (email at minimum; SMS or push if the chosen service
      offers it free).
- [ ] Test alert: stop the api container (`docker stop magiklead_api`),
      wait 3 monitor cycles, receive a downtime notification.
      Restart, receive an "up again" resolution notification.
      Repeat for the frontend container.
- [ ] Monitor URL/page handle stored in the operator's runbook so
      the dashboard is findable later.

## Modules touched

- Operational only — no code changes.

## Test prior art

- None.

## Out of scope

- Public status page (status.magiklead.com or similar) — defer.
- Synthetic transaction monitoring (full sign-up flow on a
  schedule) — defer; the Playwright suite from
  [#14](./14-playwright-e2e-suite.md) covers similar ground on a
  PR/nightly cadence.
- Multi-region pingers — defer.
- Wiring the monitor's alerts into Sentry / Slack / PagerDuty —
  defer; email is the v1 channel.
