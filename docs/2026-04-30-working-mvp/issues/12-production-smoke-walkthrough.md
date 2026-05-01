# 12 — Production smoke walkthrough

**Type**: HITL — operator runs through the flow in a browser end-to-end.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#1](./01-lazy-tenant-bootstrap.md), [#5](./05-frontend-on-hetzner.md), [#6](./06-production-data-migration.md), [#7](./07-sentry-error-tracking.md), [#8](./08-resend-transactional-email.md), [#9](./09-sender-domain-auth.md), [#10](./10-uptime-monitoring.md), [#11](./11-postgres-backups.md)

## What to build

The operator walks the smoke path on the deployed production site, end to end, against real data. This is the validation gate that decides whether MagikLead is fit for the next-step launch PRD. Any broken step gets fixed before that follow-up opens.

This slice is verification, not new construction.

## Acceptance criteria

- [ ] **Sign up** at `https://<domain>` via Clerk hosted page using a real email — bootstrap fires on first protected request, `users` + `tenants` + `user_tenants` + free `subscriptions` rows materialise. No 5xx errors in browser network tab.
- [ ] **Onboarding** completes: paste a website URL → AI analysis runs → pick plays → first campaign created. The campaign appears in `/campaigns`.
- [ ] **Search**: `/leads` search "VP Sales" returns real persons (from EDGAR/Wikidata/TeamPages, *not* fixtures). Verified-email filter has signal — at least some results have verified emails.
- [ ] **Save**: save 2–3 leads from the search results; they appear in the saved-leads view.
- [ ] **Privacy erasure**: in a different browser without auth, submit `/privacy/erasure` for the operator's email. Click the confirmation link in the operator's *real* inbox (not MailHog). Erasure completes; the operator's data is removed from `tenant_leads` / `users` etc.
- [ ] **Admin**: add operator email to `ADMIN_EMAILS` env, restart api. `/conflicts` and `/persons/<id>` are reachable; admin merge/reject/delete actions work.
- [ ] **Error tracking** (issue #7 verification): trigger an intentional test error; confirm Sentry receives it with the expected user ID and tenant ID attached.
- [ ] **Uptime monitor** (issue #10 verification): stop and restart the api container; confirm the monitor pages and resolves.
- [ ] **Backups** (issue #11 verification): confirm last night's backup exists on the Storage Box; spot-check a row count from a restored copy.
- [ ] **Email auth** (issue #9 verification): an erasure confirmation email's headers in Gmail show SPF/DKIM/DMARC all PASS.
- [ ] **Operator sign-off**: this is fit to dogfood for days. The follow-up launch PRD (production Clerk + Stripe swap, public launch) can open.
- [ ] Any broken step is captured as a follow-up issue in this directory or fixed in place before sign-off.

## Modules touched

- All of them — this is integration verification, not new code.

## Test prior art

- The predecessor plan's T03 walkthrough at `docs/2026-04-29-zero-friction-smoke/plan.md` §T03 is the same shape with a real-domain twist.

## Out of scope

- Production Clerk + Stripe key swap — that's the next PRD per the working-mvp PRD's "What comes next" section.
- Public launch (marketing, customer acquisition, status page) — that's the next-next PRD.
- Load testing, performance benchmarking — defer; v1 expects single-digit concurrent users (the operator).
- Fixing every minor issue surfaced during the walk if the operator decides the slice can ship with known issues — operator's call to triage.
