# 11 — Uptime monitoring

**Type**: HITL — needs a monitor-service account + URL configuration.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#5 — Hetzner VPS + domain + api online](./05-hetzner-vps-and-api.md)

## What to build

Wire an external uptime monitor (Better Stack, UptimeRobot, or operator's choice with a free tier) to ping `https://api.<domain>/health` every 60 seconds and notify the operator on three consecutive failures. After this slice, the operator gets paged within ~3 minutes if the production stack goes down.

## Acceptance criteria

- [ ] Monitor account exists on a free-tier-acceptable plan (UptimeRobot's free tier covers up to 50 monitors; Better Stack's free tier covers a handful).
- [ ] One monitor configured: target `https://api.<domain>/health`, expected response `200 OK` with body containing `ok`, interval 60s, failure threshold 3 consecutive.
- [ ] Notification channel configured to reach the operator (email at minimum; SMS or push if the chosen service offers it free).
- [ ] Test alert: stop the api container (`docker stop magiklead_api`), wait 3 monitor cycles, receive a downtime notification. Restart, receive an "up again" resolution notification.
- [ ] Monitor URL/page handle stored in the operator's runbook so the dashboard is findable later.

## Modules touched

- Operational only — no code changes.

## Test prior art

- None.

## Out of scope

- Public status page (status.magiklead.com or similar) — defer.
- Synthetic transaction monitoring (full sign-up flow on a schedule) — defer.
- Multi-region pingers — defer; a single-region pinger is enough for v1.
- Wiring the monitor's alerts into Sentry / Slack / PagerDuty — defer; email is the v1 channel.
