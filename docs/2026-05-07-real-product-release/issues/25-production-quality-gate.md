# 25 — Production quality gate

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#14 — Playwright E2E suite](./14-playwright-e2e-suite.md), [#16 — Hetzner VPS + api online](./16-hetzner-vps-and-api.md), [#17 — Frontend on Hetzner](./17-frontend-on-hetzner.md), [#18 — Production data migration](./18-production-data-migration.md), [#19 — Sentry error tracking](./19-sentry-error-tracking.md), [#20 — Resend transactional email](./20-resend-transactional-email.md), [#21 — Sender domain auth](./21-sender-domain-auth.md), [#22 — Uptime monitoring](./22-uptime-monitoring.md), [#23 — Postgres nightly backups](./23-postgres-backups.md), [#24 — Production Clerk + Stripe keys swap](./24-production-keys-swap.md)

## What to build

Phase 7 of the PRD — re-run the Playwright suite from #14 against
production, run the manual pre-launch runbook against production
with screenshots + DB-state captured to an artefact, and verify
the three operational invariants are live: Sentry alerting fires
on an intentional 500, the uptime monitor pages on a forced
outage, the Postgres backup restore was tested at least once.

After this slice, all five "ready to flip public" exit criteria
from the PRD are demonstrably true. The seven-day public clock
([#26](./26-seven-day-public-launch.md)) can start.

## Acceptance criteria

- [ ] Playwright suite from [#14](./14-playwright-e2e-suite.md)
      retargeted at production (env-driven base URL +
      production-ready test fixtures); all fifteen flows green.
      Run artefacts attached to this issue or stored under
      `docs/2026-05-07-real-product-release/artefacts/<date>/`.
- [ ] Manual pre-launch runbook: operator walks the same fifteen
      flows in a browser against `magiklead.com`, capturing a
      screenshot per flow and a JSON dump of relevant DB rows
      (counts of `tenants`, `campaigns`, `campaign_leads`,
      `email_events`). Artefacts stored under the same dated
      `artefacts/` directory.
- [ ] **Sentry alert verification** (PRD exit criterion #5):
      trigger an intentional 500 in production (temporary
      `/sentry-test` endpoint, removed after). Confirm:
  - Sentry receives the event within 60s,
  - the alert email arrives in the operator's inbox.
  Record time-to-alert in this file's "Outcome" section.
- [ ] **Uptime monitor verification** (PRD exit criterion #5):
      stop the api container on the VPS; wait for the monitor
      to fire; confirm the operator receives the downtime alert;
      restart; confirm the "up again" notification. Record times
      in this file.
- [ ] **Backup restore verification** (PRD exit criterion #5):
      already done in [#23](./23-postgres-backups.md); this slice
      cross-references the outcome and confirms it within the
      last 48 hours.
- [ ] **Repo cleanliness check** (PRD exit criterion #4):
      `grep -r "TODO\|FIXME\|panic\|not implemented" backend/internal/{worker,gmail,handler}/ frontend/src/{app,lib}/`
      returns clean. Any remaining hits are either justified
      (existing TODOs explicitly accepted with a note in this
      file) or fixed in flight.
- [ ] All five PRD exit criteria documented in this file's
      "Outcome" section as PASS with date-stamps. Any FAIL pauses
      Phase 8 from starting; the gap is fixed and the relevant
      check re-run.
- [ ] Operator sign-off recorded as the final line of "Outcome":
      "Production quality gate passed on YYYY-MM-DD;
      [#26](./26-seven-day-public-launch.md) can start."

## Modules touched

- Verification + artefact capture only. No new code.
- Temporary `/sentry-test` endpoint added and removed in the same
  PR.

## Test prior art

- [#14 — Playwright E2E suite](./14-playwright-e2e-suite.md) is
  re-used here.
- The working-MVP
  [#13 — Production smoke walkthrough](../../2026-04-30-working-mvp/issues/13-production-smoke-walkthrough.md)
  is the predecessor manual-walkthrough shape; this slice is its
  real-product successor.

## Out of scope

- The seven-day endurance run itself —
  [#26](./26-seven-day-public-launch.md).
- Performance / load testing — defer.
- Public-launch marketing announcements — defer.
