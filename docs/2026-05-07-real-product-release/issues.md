# Issues — 2026-05-07-real-product-release

Source: [prd.md](./prd.md)

This PRD supersedes `../2026-04-30-working-mvp/prd.md`. Working-MVP
issues #5–#13 are absorbed into Phase 6 of this tracker as
self-contained copies. Working-MVP issues #1 (lazy tenant bootstrap)
and #4 (local smoke walkthrough) remain on the working-MVP tracker
and gate Phase 0 of this PRD.

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Phase 0 unblock fixes](./issues/01-phase-0-unblock.md) | None |
| [x]  | 2 | [Suppression module + schema migrations](./issues/02-suppression-module.md) | [#1](./issues/01-phase-0-unblock.md) |
| [x]† | 3 | [Gmail OAuth real send](./issues/03-gmail-oauth-send.md) | [#2](./issues/02-suppression-module.md) |
| [x]† | 4 | [Reply detection via Gmail History API](./issues/04-reply-detection.md) | [#3](./issues/03-gmail-oauth-send.md) |
| [x]† | 5 | [Bounce detection](./issues/05-bounce-detection.md) | [#4](./issues/04-reply-detection.md) |
| [x]† | 6 | [Unsubscribe end-to-end](./issues/06-unsubscribe.md) | [#2](./issues/02-suppression-module.md), [#3](./issues/03-gmail-oauth-send.md) |
| [x]† | 7 | [PDL integration + canonical write-through](./issues/07-pdl-integration.md) | [#1](./issues/01-phase-0-unblock.md) |
| [x]† | 8 | [Settings page real data](./issues/08-settings-real-data.md) | [#3](./issues/03-gmail-oauth-send.md) |
| [x]† | 9 | [Per-campaign metrics dashboard](./issues/09-campaign-metrics.md) | [#3](./issues/03-gmail-oauth-send.md), [#4](./issues/04-reply-detection.md), [#5](./issues/05-bounce-detection.md), [#6](./issues/06-unsubscribe.md) |
| [x]† | 10 | [Live Stripe billing](./issues/10-live-stripe-billing.md) | [#8](./issues/08-settings-real-data.md) |
| [x]† | 11 | [Account export + delete](./issues/11-account-export-delete.md) | [#1](./issues/01-phase-0-unblock.md) |
| [x]† | 12 | [UI/UX design review pass](./issues/12-ui-ux-review.md) | [#2](./issues/02-suppression-module.md), [#3](./issues/03-gmail-oauth-send.md), [#4](./issues/04-reply-detection.md), [#5](./issues/05-bounce-detection.md), [#6](./issues/06-unsubscribe.md), [#7](./issues/07-pdl-integration.md), [#8](./issues/08-settings-real-data.md), [#9](./issues/09-campaign-metrics.md), [#10](./issues/10-live-stripe-billing.md), [#11](./issues/11-account-export-delete.md) |
| [x]  | 13 | [D-bar flow walkthrough](./issues/13-d-bar-flow-walkthrough.md) | [#12](./issues/12-ui-ux-review.md) |
| [x]  | 14 | [Playwright E2E suite (15 flows)](./issues/14-playwright-e2e-suite.md) | [#13](./issues/13-d-bar-flow-walkthrough.md) |
| [ ]  | 15 | [Local 3-day real test](./issues/15-local-3-day-real-test.md) | [#14](./issues/14-playwright-e2e-suite.md) |
| [ ]  | 16 | [Hetzner VPS + api online](./issues/16-hetzner-vps-and-api.md) | [#15](./issues/15-local-3-day-real-test.md) |
| [ ]  | 17 | [Frontend on Hetzner](./issues/17-frontend-on-hetzner.md) | [#16](./issues/16-hetzner-vps-and-api.md) |
| [ ]  | 18 | [Production data migration](./issues/18-production-data-migration.md) | [#16](./issues/16-hetzner-vps-and-api.md) |
| [ ]  | 19 | [Sentry error tracking](./issues/19-sentry-error-tracking.md) | [#16](./issues/16-hetzner-vps-and-api.md) |
| [ ]  | 20 | [Resend transactional email](./issues/20-resend-transactional-email.md) | [#16](./issues/16-hetzner-vps-and-api.md) |
| [ ]  | 21 | [Sender domain auth (SPF/DKIM/DMARC)](./issues/21-sender-domain-auth.md) | [#20](./issues/20-resend-transactional-email.md) |
| [ ]  | 22 | [Uptime monitoring](./issues/22-uptime-monitoring.md) | [#16](./issues/16-hetzner-vps-and-api.md) |
| [ ]  | 23 | [Postgres nightly backups](./issues/23-postgres-backups.md) | [#16](./issues/16-hetzner-vps-and-api.md), [#18](./issues/18-production-data-migration.md) |
| [ ]  | 24 | [Production Clerk + Stripe keys swap](./issues/24-production-keys-swap.md) | [#16](./issues/16-hetzner-vps-and-api.md), [#10](./issues/10-live-stripe-billing.md) |
| [ ]  | 25 | [Production quality gate](./issues/25-production-quality-gate.md) | [#14](./issues/14-playwright-e2e-suite.md), [#16](./issues/16-hetzner-vps-and-api.md), [#17](./issues/17-frontend-on-hetzner.md), [#18](./issues/18-production-data-migration.md), [#19](./issues/19-sentry-error-tracking.md), [#20](./issues/20-resend-transactional-email.md), [#21](./issues/21-sender-domain-auth.md), [#22](./issues/22-uptime-monitoring.md), [#23](./issues/23-postgres-backups.md), [#24](./issues/24-production-keys-swap.md) |
| [ ]  | 26 | [7-day public real launch](./issues/26-seven-day-public-launch.md) | [#25](./issues/25-production-quality-gate.md) |

† Implementation + automated tests complete; one acceptance
criterion (manual local end-to-end with a real Gmail account)
deferred until [#8](./issues/08-settings-real-data.md) ships the
"Connect Gmail" UI. See #3's "Manual-verification prerequisites"
section. #4, #5, #6, and #9 inherit the same prerequisite chain.
#7's manual step requires a real `PDL_API_KEY` plus the onboarding
flow; flipped once the operator walks it. #8 ships the "Connect
Gmail" UI plus the real `/settings` payload; its own
manual-verification step (visit `/settings`, disconnect, reconnect
via OAuth) and the inherited #3 single-send walk both flip once
the operator runs the OAuth dance against a real Google Cloud
Console client. #9 additionally needs
`UNSUBSCRIBE_SIGNING_SECRET` set in this devpod's `.env.backend`
so the api process can boot (the gate was added by #6 but this
devpod's env file was never refreshed).
#10's three operator-dashboard criteria (Live-mode prices, promo
code, Live webhook endpoint) and the manual end-to-end against
Stripe Live all belong to the production cutover and flip during
[#24](./issues/24-production-keys-swap.md) /
[#25](./issues/25-production-quality-gate.md) /
[#26](./issues/26-seven-day-public-launch.md).
#11's manual local verification (export → unzip → spot-check;
delete → Clerk dashboard confirms user gone) needs a real Clerk
dev account + Gmail OAuth connection; flips during
[#13](./issues/13-d-bar-flow-walkthrough.md) /
[#15](./issues/15-local-3-day-real-test.md).
#12 is the code-level UI/UX pass — copy demo-tells fixed, three
follow-ups captured in the issue's Outcome section (SMTP-vs-Gmail
on campaign page, silent error-swallow on dashboard/campaigns,
`alert()`-style error UX). Live-browser visual eyeball still owed.
