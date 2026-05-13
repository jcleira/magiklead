# 04 — Local smoke walkthrough

**Type**: HITL — operator runs the full flow on the local devpod.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#1 — Lazy tenant bootstrap](./01-lazy-tenant-bootstrap.md), [#2 — Devpods seed snapshot](./02-devpods-seed-snapshot.md), [#3 — Run real ingest pipeline](./03-real-ingest-run.md)

## What to build

The operator walks the full sign-up → onboarding → search → save → erasure → admin loop on the local devpod (`mvp.magiklead.localhost`), end to end. This is the gate that says "the product works as a product" before any production deploy work begins. Anything broken in the devpod is an order of magnitude harder to diagnose behind Caddy + LetsEncrypt + DNS, so it gets fixed here first.

This slice is verification, not new construction. It also subsumes #1's last manual checkbox ("fresh devpod, sign in via Clerk, rows appear") — completing this issue closes that gate as a side effect.

## Acceptance criteria

- [ ] **Sign up** at `http://mvp.magiklead.localhost` via Clerk hosted page using a test email — bootstrap fires on the first protected request, `users` + `tenants` + `user_tenants` + free `subscriptions` rows materialise. No 5xx errors in the network tab. Verify via `devpods psql`.
- [ ] **Onboarding** completes: paste a website URL → AI analysis runs (Anthropic key configured in `.env.backend`) → pick plays → first campaign created. The campaign appears in `/campaigns`.
- [ ] **Search**: `/leads` search "VP Sales" returns real persons (from the EDGAR/Wikidata/TeamPages ingest), not just `(devpod-fixture)` rows. Verified-email filter is allowed to be empty for this walk — that signal lands with [#14](./14-email-finder-source.md).
- [ ] **Save**: save 2–3 leads from the search results; they appear in the saved-leads view.
- [ ] **Privacy erasure**: in a different browser without auth, submit `/privacy/erasure` for a test email. Click the confirmation link in MailHog (`http://mail-mvp.magiklead.localhost`). Erasure completes; the test email's data is removed from `tenant_leads` / `users` etc.
- [ ] **Admin gate**: add the operator's email to `ADMIN_EMAILS` in `.env.backend`, restart api. `/conflicts` and `/persons/<id>` are reachable; admin merge/reject/delete actions work.
- [ ] **Closes #1's manual gate**: the lazy-tenant-bootstrap issue's last unchecked acceptance row (`Manual: devpods up on a fresh devpod...`) is checked off as a side effect.
- [ ] Any broken step gets a follow-up issue in this directory or is fixed in place before sign-off — production deploy work ([#5 onward](./05-hetzner-vps-and-api.md)) does not begin until this is green.

## Modules touched

- All of them — this is integration verification on the devpod, not new code.

## Test prior art

- The deployed-site equivalent is [#13 — Production smoke walkthrough](./13-production-smoke-walkthrough.md). This is the local mirror; #13 re-verifies on real infra after deploy.
- The predecessor plan's T03 walkthrough at `docs/2026-04-29-zero-friction-smoke/plan.md` §T03 is the same shape against the local devpod.

## Out of scope

- Real transactional email — MailHog is the local mailer. Real Resend send happens in [#9](./09-resend-transactional-email.md) and gets re-verified in [#13](./13-production-smoke-walkthrough.md).
- Production-grade auth/SSL/domain routing — covered by [#5](./05-hetzner-vps-and-api.md), [#6](./06-frontend-on-hetzner.md).
- Verified-email filter signal — depends on [#14](./14-email-finder-source.md); acceptable to be empty for this local walk.
- Production smoke walkthrough on real infra — that's [#13](./13-production-smoke-walkthrough.md).
- Sentry / monitoring / backups — production-only concerns ([#8](./08-sentry-error-tracking.md), [#11](./11-uptime-monitoring.md), [#12](./12-postgres-backups.md)).
