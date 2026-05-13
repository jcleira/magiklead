# Real Product Release — PRD

**Date:** 2026-05-07
**Supersedes:** `../2026-04-30-working-mvp/prd.md` (absorbs its remaining issues #5–#13 into Phase 6 below; adds the customer-facing gaps that the working-MVP smoke walk surfaced).
**Prior attempts:** `../2026-04-25-initial-release/plan.md`, `../2026-04-29-zero-friction-smoke/plan.md`, `../2026-04-30-working-mvp/prd.md`.

---

## Problem Statement

The operator has now run two MVP cycles back-to-back and both produced shells, not products. The third attempt (the working-MVP smoke walk on 2026-05-07) revealed exactly why: the settings page renders hard-coded "Free" and "100 leads/mo" instead of real subscription data, the Gmail OAuth send path is a `TODO` stub, reply detection is fully absent (so a recipient who replies "interested" still receives follow-up #2), every prior PRD had per-task acceptance criteria with no forcing function that proved the full flow worked under real conditions, and "the operator dogfoods and decides it's ready" turned out to be a gate the operator could pass while the product was still broken.

The operator now wants a real product, not a third shell. Concretely: a public site at `magiklead.com` that the operator personally uses to run real outreach for two of their own products (magikshot.com and magiktext), where external sign-ups during the validation window pay real money to use it, and where there is a hard, observable gate that prevents any "looks like demo" failure mode from reaching launch. If a tester or a paying customer ever calls the product shit, the project gets killed — so the bar this PRD has to clear is that nobody can.

This means everything that is currently a stub, a hard-coded value, or a deferred-for-later concession has to be real before launch. There is no "we'll fix it after" budget left.

## Solution

Take MagikLead from "shell" to "real product on `magiklead.com`, public sign-up live from day 1, used by the operator for real outreach on magikshot and magiktext concurrently."

The solution has four shapes:

1. **Close the customer-facing gaps the audit surfaced.** Ship the real settings page (workspace name, plan, usage from the API), the real Gmail OAuth send path (replacing the `TODO` in `gmail/sender.go`), reply detection via the Gmail History API (per-mailbox polling every two minutes, halt the sequence on any reply, capture the message-id chain on send so replies match cleanly), bounce handling (hard bounce halts; soft bounce halts after three consecutive), unsubscribe (RFC 8058 one-click HTTPS plus mailto, per-tenant + global suppression), per-campaign metrics (leads / sent / replied / bounced / unsubscribed — no opens, no clicks), and live Stripe billing (real prices in Stripe Live mode, real webhook, real money; testers comp'd via a 100%-off promo code).

2. **Make the product actually find leads.** Replace the placeholder "paid email-finder later" deferral with a real People Data Labs integration ($98/mo, 5k credits/mo). Layer the existing canonical graph (EDGAR + Wikidata + TeamPages) underneath as a free quality cache: when a tester searches, results come from PDL but flow through `persons` and `emails`, so subsequent identical lookups across tenants are free. The existing onboarding flow (paste website → AI plays) feeds PDL filters; an ICP-description and structured-filter UI sit alongside it for refinement.

3. **Apply a three-layered forcing function before launch.** A Playwright end-to-end suite that exercises all fifteen D-bar flows on every PR and nightly, catching feature regression. A pre-launch human runbook walked once within forty-eight hours of any deploy, with screenshots and DB-state captured to an artefact, catching demo-tells the automation cannot see. And the apex gate: a real seven-day concurrent run of magikshot and magiktext outreach campaigns on the public production site, with at least one hundred prospects each, at least one reply detected and the sequence halted, at least one bounce handled, and zero emails sent to leads who replied (binary, hard-blocking — one false positive resets the seven-day clock).

4. **Sequence the work so the forcing function actually works.** Eight phases, in order, no parallelism across phase boundaries: build the gaps, review UI/UX, validate every flow against operator expectation, automate the quality gate locally, run a three-day local real test (real Gmail, real PDL, real prospects, real replies — only the URL stays on `mvp.magiklead.localhost`), wire production, re-run the quality gate against production, then start the seven-day public clock. Any P1 bug at any phase pauses the relevant clock; P1 specifically includes any email sent to a replied lead, send failure rate above ten percent, settings page showing wrong data, sequence not pausing on reply, bounce or unsubscribe silently dropped, privacy erasure broken, and site unreachable.

After this PRD lands, the operator has a real product, used by them, with public sign-up live, where the validation evidence (E2E artefacts, runbook outputs, and the seven-day campaign metrics with zero replied-lead violations) is the gate that says "ready" — not the operator's gut.

## User Stories

### Sign-up, profile, and workspace

1. As a new user, I want to sign up at `magiklead.com` via Clerk and land on a working dashboard, so that the first impression is not a sign-up wall failing.
2. As a new user, I want my workspace, role, and free-tier subscription created automatically on first protected request, so that I never see a half-bootstrapped state or a terminal-only setup.
3. As a user, I want the settings page to display my real workspace name, my real plan, my real lead-quota usage, and my real connected email accounts, so that nothing on screen looks fake or hard-coded.
4. As a user, I want to disconnect a connected Gmail account and reconnect a different one, so that I can change which mailbox I send through.
5. As a user, I want to delete my account and have all my data erased on confirmation, so that I retain control over my data.
6. As a user, I want to export all my data (campaigns, leads, sequences, sends, replies) as a downloadable archive, so that I am not locked in.

### Lead discovery

7. As a user, I want to paste my company's website and get a list of plausible buyer ICPs, so that the onboarding flow does the targeting work I came here to avoid.
8. As a user, I want to refine the inferred ICP with a free-text description ("Heads of Marketing at B2B SaaS, 50–500 employees, US+EU"), so that I can correct an inference that missed.
9. As a power-user, I want a structured filter UI (job title, company size, industry, geography), so that I can run precise queries when I know what I want.
10. As a user, I want lead search results to come from a real B2B people database covering at least a billion persons, so that my niche personas are actually findable.
11. As a user, I want every lead in my results to carry a verified email address (or be clearly marked unverified), so that my campaigns do not bounce themselves into a spam trap.
12. As a user, I want to save leads from search results to a campaign, with the saved-lead count counted against my plan's monthly quota, so that I see exactly how much of my plan I have used.
13. As an operator, I want lookups cached across all tenants in the canonical `persons` and `emails` tables, so that the system pays the data provider once per person, regardless of how many users save them.

### Email connect

14. As a user, I want to connect my Google Workspace mailbox via OAuth, granting send + read scopes, so that the system sends from my real address and detects my replies.
15. As a user, I want my OAuth tokens stored encrypted at rest and refreshed automatically, so that I do not have to reconnect every week.
16. As a user, I want a clear status indicator in the UI for each connected mailbox (connected / token-expired / scope-missing), so that I know when something needs my attention before sends start failing.
17. As a user, I want a clear error if my employer's Google Workspace blocks third-party OAuth grants, so that I do not waste time on a broken integration without knowing why.

### Sequences and sending

18. As a user, I want to generate a multi-step sequence (subject + body, with merge-tag personalisation) from my campaign's ICP and play, so that I am not writing cold copy from scratch.
19. As a user, I want to edit each step of a generated sequence (subject, body, delay days), so that I am not stuck with whatever the AI produced.
20. As a user, I want to start a campaign and have the worker send step 1 to all active leads from my connected Gmail, so that sending happens automatically.
21. As a user, I want each subsequent step to fire after the configured delay (with a small randomised jitter to avoid send-pattern detection), so that the sequence runs on its own.
22. As a user, I want the worker to record every send as an event with the Gmail message-id, so that downstream reply matching is exact.
23. As a user, I want to pause and resume a campaign at any time, so that I can stop sends quickly if I notice something wrong.

### Reply detection

24. As a user, I want the system to poll my connected Gmail every two minutes for new messages, so that replies are detected within a small, predictable latency window.
25. As a user, I want a reply to any sent step to mark the lead as replied, set the campaign-lead status to `replied`, and halt all future sends to that lead immediately, so that nobody who responded to me ever gets a follow-up.
26. As a user, I want to manually re-engage a replied lead via a button in the UI, so that "they replied with an out-of-office, not a real reply" is recoverable.
27. As an operator, I want zero emails sent to leads who have already replied — a hard, binary, non-negotiable invariant — so that the product is never accused of bad-faith follow-up.

### Bounce and unsubscribe

28. As a user, I want a hard-bounce response (5xx delivery error or DSN) to mark the lead as bounced, halt the sequence, and add the address to the suppression list, so that I am not training my Gmail's reputation on undeliverable addresses.
29. As a user, I want a soft-bounce response (4xx) to halt only after three consecutive soft bounces, so that a transient mailbox-full does not lose me the lead permanently.
30. As a user, I want every outbound message to include both a `mailto:` and an RFC 8058 one-click HTTPS unsubscribe header, so that Gmail's deliverability checks are satisfied and recipients can unsubscribe in one click.
31. As a user, I want a recipient's unsubscribe to add them to my per-tenant suppression list, so that they are never emailed again from any of my campaigns.
32. As an operator, I want unsubscribed addresses also added to a global suppression list, so that they are never emailed from any tenant on the platform (CAN-SPAM compliance).

### Per-campaign metrics

33. As a user, I want each campaign's detail page to show: total leads, total sent (with per-step breakdown), total replied, total bounced, total unsubscribed, so that I can tell at a glance whether the campaign is working.
34. As a user, I want these numbers to update in near-real-time as the worker progresses, so that I do not have to refresh and wait.

### Billing

35. As a user, I want a free tier of one hundred leads per month, so that I can try the product without paying.
36. As a user, I want to upgrade via a Stripe Checkout flow when I hit my quota, so that the friction from "free" to "paid" is small.
37. As a user, I want the upgrade webhook to lift my quota within seconds of payment success, so that I do not get stuck after paying.
38. As a user, I want my plan and current usage shown on the settings page, so that I always know where I stand.
39. As an operator, I want a 100%-off promo code I can hand out to test users during the validation window, so that I can comp friends and external testers without disabling billing for everyone.
40. As an operator, I want the entire billing flow to run against Stripe Live mode from day one, so that the most failure-prone integration is exercised against real money before the first real customer.

### Privacy and admin

41. As a non-authenticated visitor, I want to submit a privacy-erasure request for an email address and confirm via a link in my inbox, so that GDPR right-to-erasure is honoured.
42. As an admin (operator), I want to view conflicts in the canonical graph and take merge / reject / delete actions, so that data-quality issues from automated ingestion can be cleaned up.
43. As an admin (operator), I want my email matched against `ADMIN_EMAILS` to gate the admin routes, so that customers cannot reach admin tooling.

### UI/UX and flow validation

44. As an operator, I want every screen reviewed against the design tokens before any deploy, so that no "demo-tell" (skeleton states, ugly copy, wrong fonts, dead clicks) ships to production.
45. As an operator, I want to walk every D-bar user flow end-to-end on the local devpod with real data and explicitly mark each one as matching expectation, so that no flow ships unverified.

### Quality gate

46. As an operator, I want a Playwright E2E suite that exercises all fifteen D-bar flows on every pull request and nightly against a real Clerk dev account, real Gmail OAuth on a test workspace, real Stripe live-mode (with the test promo code), and real PDL credits, so that feature regression is caught automatically.
47. As an operator, I want a pre-launch runbook walked manually within forty-eight hours of any deploy, with screenshots and DB-state captured to an artefact, so that demo-tells the automation cannot see are caught before they reach customers.
48. As an operator, I want a three-day local real test (real Gmail, real PDL, ~100 prospects, real replies, real bounces) on the local devpod before any production deploy, so that real-product bugs are surfaced without exposing them publicly.
49. As an operator, I want a seven-day concurrent run of real magikshot and magiktext campaigns on production, with hard exit criteria (zero replied-lead violations, at least one reply detected, at least one bounce handled, no Sentry P1, settings correct throughout), so that the validation evidence is observable rather than gut-felt.
50. As an operator, I want any P1 bug to pause the relevant clock and force a restart of the affected seven-day or three-day window, so that no shortcuts past the gate are possible.

### Production deploy and launch

51. As an operator, I want the api + worker + frontend deployed onto a single Hetzner VPS behind Caddy with auto-SSL, so that infrastructure is something I control.
52. As an operator, I want Sentry capturing every error with traces and breadcrumbs, so that I can debug production issues without SSH.
53. As an operator, I want an external uptime monitor pinging the public health endpoints every minute, so that downtime is detected even when I am asleep.
54. As an operator, I want nightly Postgres + object-storage backups with a tested restore procedure, so that data loss is recoverable.
55. As an operator, I want a working Resend integration on `magiklead.com` for transactional system mail (privacy-erasure links, billing receipts, password resets, alerts), so that system mail is reliable and separate from the per-tester outbound channel.
56. As an operator, I want SPF / DKIM / DMARC configured for the sender domain, so that transactional mail lands in inboxes.
57. As an operator, I want the Clerk dev keys swapped for production keys before public launch, with the JWT template and webhook configured on the production instance, so that the public site uses production identity.
58. As an operator, I want public sign-ups enabled from day one of launch (no invite-only gate), so that the validation includes real-world traffic from strangers.

## Implementation Decisions

### Email architecture

Per-tester outbound goes through Gmail OAuth — each connected account sends through its own mailbox, so deliverability and reputation are owned by the tester. Resend on `magiklead.com` handles transactional mail only (privacy-erasure links, billing receipts, password resets, operator alerts). Outlook OAuth and custom SMTP are deferred. Cold outbound through a shared transactional provider was considered and rejected: provider TOS would suspend the account on complaint thresholds, and the shared `magiklead.com` domain reputation would be destroyed within weeks of cold sends, taking transactional deliverability down with it.

### Reply, bounce, and unsubscribe detection

Gmail History API polling per connected mailbox, two-minute cadence, stateless cursor stored on `gmail_accounts.last_history_id`. Pub/Sub push notifications were considered and deferred — they require GCP infrastructure that is not yet in scope, and minutes-of-latency is acceptable for cold outbound. Each outbound send via the Gmail API captures the returned `messageId` on the `email_events` row in a new dedicated `gmail_message_id` column. On poll, new messages with `In-Reply-To` matching a stored message-id mark the campaign-lead replied and halt the sequence. Bounces (Gmail DSN format) increment `bounce_count`; hard bounce halts immediately, three consecutive soft bounces halts. Unsubscribe ships with both a `mailto:` header and an RFC 8058 one-click HTTPS endpoint at `/api/v1/public/unsubscribe?token=<signed-jwt>`. Suppression is per-tenant plus a global table.

### Lead-discovery architecture

People Data Labs is the spine: API-first, pay-per-credit, designed for embedding in other SaaS products, $98/mo entry plan covers the validation window with multiples of headroom. Apollo and ZoomInfo were considered; Apollo's per-user pricing is hostile to multi-tenant resellers, ZoomInfo is enterprise-priced and out of scope. The existing canonical graph (EDGAR + Wikidata + TeamPages) is preserved as a free quality layer underneath PDL — every lookup consults the canonical first, falls through to PDL on miss, and writes the PDL result back to the canonical so subsequent identical lookups across tenants cost nothing. Stale-after-90-days policy on cached emails. The free tier (one hundred leads per month) is metered against unique persons saved per tenant; multi-step sends to a single saved lead cost one quota unit, not many.

### Suppression

A unified `suppression` deep module exposes `IsSuppressed(tenant, email) → bool` and event-recording functions. The worker calls `IsSuppressed` immediately before any send; the poller calls the event recorders post-classification. Reply, hard bounce, three-consecutive-soft-bounce, and unsubscribe all funnel through this module, ensuring one source of truth for "do not email this person."

### Billing

Stripe Live mode from day one. Real prices created for Free / Starter / Growth / Scale (price IDs already exist as placeholders in `.env.backend`; replaced with live values during Phase 6). Live webhook configured at `api.magiklead.com`. Testers comp'd via a single 100%-off Stripe promo code distributed manually. Dev Stripe is used only for the local three-day test; the production seven-day clock runs on Live mode against real cards.

### Module shape

Four deep modules carry most of the new functionality:

- **`gmail/poller`** — per-mailbox History API polling. Single interface: takes a mailbox identifier and a cursor, returns classified events. Stubbable seam via the Gmail SDK interface, so unit tests run without hitting Google.
- **`gmail/sender`** — finishes the existing `TODO`. Single interface: sends a message via the Gmail API on a connected account, returns the `messageId`.
- **`leads/pdl`** — PDL search and enrich. Single interface for search, single for enrich, both writing through to the canonical graph. Stubbable via an HTTP-client seam.
- **`suppression`** — already described above. Centralises the "do-not-email" decision and writes.

Shallow modules are the handlers (settings, metrics, public unsubscribe), the worker poller tick (alongside the existing sender tick), and the frontend pages (settings, metrics dashboard, gmail-connect status, badges).

Existing modules modified: `worker/sender` swaps SMTP for Gmail OAuth and gates every send through `suppression`; `handler/billing` extends for live webhook and plan-upgrade flow; `handler/gmail` extends OAuth scopes to include History API access; `settings/page.tsx` and `(app)/layout.tsx` replace hard-coded plan strings with API fetches.

### Schema changes

New columns: `email_events.gmail_message_id TEXT INDEX`, `gmail_accounts.last_history_id TEXT`, `gmail_accounts.last_polled_at TIMESTAMPTZ`. New table: `unsubscribes` with a `tenant_id NULL`-able foreign key (NULL = global), `email TEXT`, `reason TEXT` (manual / list-unsub / spam-complaint), `created_at TIMESTAMPTZ`, unique on `(COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'), lower(email))`.

### Phase ordering

Eight phases, no parallelism across phase boundaries. Each phase has hard gates that must be green before the next starts.

- **Phase 0 — Unblock (~1 day):** fix today's devpods CLI regression; commit today's storage scheme fix, `next.config.ts` `allowedDevOrigins`, real `CLERK_SECRET_KEY`, and `ADMIN_EMAILS` value; open new issues from this PRD.
- **Phase 1 — Customer-facing gaps (~5–7 days):** settings real data, Gmail OAuth send, reply detection, bounce, unsubscribe, PDL integration, per-campaign metrics dashboard.
- **Phase 2 — UI/UX design review (~1–2 days):** walk every screen on the local devpod with real data; fix demo-tells against the design tokens.
- **Phase 3 — Flow validation (~1–2 days):** operator manually walks every D-bar flow and marks each as matching expectation; bugs found are fixed in flight.
- **Phase 4 — Local quality gate (~1–2 days):** Playwright E2E suite for all fifteen flows, run against the local devpod with real Gmail OAuth and real PDL.
- **Phase 5 — Local 3-day real test (~3 days):** magikshot campaign on the local devpod with one hundred prospects, real Gmail send, real PDL lookups, real replies, real bounces. Any P1 bug pauses and restarts the three-day clock.
- **Phase 6 — Production wiring (~3–4 days):** absorbs remaining issues from the working-mvp PRD (Hetzner VPS, frontend on Hetzner, production data migration, Sentry, Resend, SPF/DKIM/DMARC, uptime monitor, Postgres backups), plus live Stripe prices and webhook, plus production Clerk + Stripe key swap.
- **Phase 7 — Production quality gate (~1 day):** re-run Playwright suite against production; re-run runbook against production; verify Sentry, uptime, backups all live (intentional 500 produces an alert; a backup restore is tested once).
- **Phase 8 — 7-day public real launch (~7 calendar days):** magikshot and magiktext campaigns running concurrently on production, daily metrics + Sentry review, any P1 bug pauses the seven-day clock and restarts the window.

### Exit criteria for "ready to flip public"

All five must be true. No exceptions, no "we'll fix it after."

1. All fifteen D-bar flows pass the automated E2E suite against real Clerk dev / Gmail OAuth on test workspace / Stripe live with promo code / real PDL.
2. Pre-launch runbook completed within the last forty-eight hours, with screenshots and DB-state captured.
3. Seven-day concurrent magikshot + magiktext production run completed cleanly: ≥100 prospects each, ≥1 reply detected and sequence halted, ≥1 bounce handled, settings correct throughout, no Sentry P1, **zero emails sent to replied leads**.
4. `grep -r "TODO\|FIXME\|panic\|not implemented"` on `backend/internal/{worker,gmail,handler}` and `frontend/src/{app,lib}` returns clean.
5. Sentry alerting verified (intentional 500 → alert reaches operator), uptime monitor active, Postgres backup restore tested at least once.

### Severity policy during the launch window

P1 (pauses clock, restarts window): any email sent to a replied lead, send failure rate above ten percent, settings page showing wrong data, sequence not pausing on reply, bounce or unsubscribe silently dropped, privacy erasure broken, site unreachable, Sentry-captured panic in any worker tick.

P2 (logged, fixed in flight, does not pause): cosmetic UI bugs, slow page loads (within reason), edge-case errors with clear stack traces.

P3 (post-launch backlog): nice-to-haves, feature requests, performance polish.

## Testing Decisions

### What makes a good test in this codebase

Tests exercise behaviour through a public interface, not implementation details. They use real Postgres (the devpod's Postgres) for anything that touches the database, real Clerk dev tokens for anything that touches auth, real Gmail-API stubs (via the SDK's interface seam) for the poller and sender, and real PDL-API stubs (via a `Doer`-style HTTP-client seam) for the lead-discovery module. A passing test must survive an internal refactor — if changing a function name or splitting a struct breaks tests, the tests are brittle.

### Modules with unit tests

The four deep modules carry the bulk of the test burden:

- **`gmail/poller`** — happy path (cursor advances, reply classified correctly, bounce classified correctly, unrelated message ignored), edge cases (cursor missing, history-id rotated by Gmail, expired token), and the failure paths (network error, rate limit, malformed response). Stub the Gmail SDK interface; do not hit Google.
- **`gmail/sender`** — happy path (send succeeds, returns message-id), failure paths (token expired, scope missing, message-rejected), and the post-send event recording (event row written with `gmail_message_id`).
- **`leads/pdl`** — happy path (search returns persons, enrich returns email, both written to canonical), cache hit (canonical already has the person — PDL not called), stale cache (cache older than 90 days — PDL re-called, canonical updated), failure paths (PDL rate limit, malformed response, credit exhausted).
- **`suppression`** — `IsSuppressed` returns true for replied leads, hard-bounced addresses, three-consecutive-soft-bounce addresses, per-tenant unsub, and global unsub; returns false otherwise. Event-recorders write the right rows.

Migrations are tested via round-trip: a fresh Postgres applies the up migration, exercises the new columns or table, then runs the down migration cleanly.

### End-to-end coverage

The Playwright suite under `tests/e2e/` exercises all fifteen D-bar user flows from the user-stories list above. The suite runs against a live devpod (or a staging-shaped environment in Phase 7) with real integrations. Each flow has an artefact (screenshot, DB-state dump) captured on success. The pre-launch runbook duplicates the same flows manually, capturing the same artefacts — this is intentional redundancy: the automation catches regression, the human catches demo-tells the automation cannot see.

### Prior art

`backend/internal/middleware/admin_test.go` is the pattern for middleware tests with manipulated context and stub handlers; the new poller / sender unit tests follow the same shape (stub the dependency, exercise the unit). `backend/internal/storage/s3_roundtrip_test.go` is the pattern for integration tests against a real backing service (skipped when the env var is unset); the new PDL tests use the same skip-pattern when `PDL_API_KEY` is absent. Existing `*_test.go` handler tests (e.g. `tenant_leads_test.go`, `privacy_test.go`) are the pattern for thin handler tests that exercise validation paths only — the new shallow handlers (settings, metrics, unsubscribe) follow this shape.

### Skip list

Frontend unit tests are not in scope — Playwright covers the frontend behaviour at the only level that matters (does the user flow work end-to-end). Shallow handlers are not unit-tested beyond validation paths; the integration tests in the E2E suite cover their behaviour. Database migrations beyond round-trip are not tested.

## Out of Scope

- **Outlook OAuth and custom SMTP outbound.** Gmail-only this release; testers without Gmail wait for a follow-up.
- **Open and click tracking.** Apple Mail Privacy Protection has destroyed the open signal; click tracking is useful but adds URL rewriting and an extra redirect hop, not worth the complexity for this release.
- **CSV import.** Magiklead is a lead finder, not an email automation platform — if the user already has the list, they are not the customer.
- **Pub/Sub push notifications for replies.** Two-minute polling latency is acceptable; Pub/Sub adds GCP infra that is not yet in scope.
- **Gmail send rate limiting beyond Gmail's own quotas.** Per-tester sending volume during the validation window is well below Gmail Workspace's 2,000-per-day cap; rate-limiting will land if a tester ever maxes out.
- **Multi-region high availability, WAF, SOC 2 readiness.** Single-region Hetzner VPS is the deploy target; HA / compliance is out of scope until there is a customer asking for it.
- **Audit logs of admin actions, account-deletion grace periods, account-merge / multi-tenant invites.** Standard but not first-week-blockers; deferred.
- **Crunchbase ingest.** Already deferred from prior PRDs; the source code stays in the repo for future use.

## Further Notes

### Five gaps the working-MVP smoke walk surfaced (today)

In the course of running `/code-execute-issue` against issue #4 of the working-mvp PRD on 2026-05-07, five latent issues turned up. These need to be carried forward:

1. **`CLERK_SECRET_KEY=sk_test_YOUR_CLERK_SECRET_KEY_HERE` placeholder accepted at startup.** The api startup check is "non-empty" not "valid format." Worth a small startup-format-validator follow-up.
2. **`S3_ENDPOINT` scheme handling broken.** Devpods now injects `http://minio:9000` (with scheme); the MinIO Go SDK's `New()` only accepts host:port. Fix landed in `backend/internal/storage/s3.go` (uncommitted on branch). Needs a unit test.
3. **`next.config.ts` missing `allowedDevOrigins`.** Without it, Next.js 16 dev server blocks `_next/static/*` and the React bundle never hydrates — the page renders SSR HTML but no JS runs, and typing into inputs does nothing. Fix landed (uncommitted).
4. **Working-mvp issue #3 "done" did not include `devpods seed regenerate`.** The real-ingest data (29k+ persons) was lost when the devpod was destroyed because the seed snapshot pre-dated the ingest. Re-ingested and snapshot regenerated; the process gap is "any data-changing operation must regenerate the snapshot before sign-off."
5. **Devpods CLI rebuilt today (2026-05-07 11:18 local) and now cannot find its compose tree.** The locator walks up from the binary path and cwd looking for `compose/traefik.yml`; the new binary at `~/.local/bin/devpods` has neither alongside it. Likely an embed regression in devpods itself; tracked separately.

These all become Phase 0 work items.

### Why this PRD's exit criteria are different from the prior three

The first PRD had per-task acceptance ("this route returns 401 cleanly", "test send arrives") with no end-to-end gate. The second PRD was a developer-convenience sprint (lazy bootstrap, seed fixture) that did not validate the product solved a customer problem. The third PRD said "operator dogfoods and decides it's ready" — a gate the operator could pass while reply detection was absent and the settings page was hard-coded.

This PRD's gate is observable, redundant, and adversarial to its own author. The seven-day production run with hard binary criteria (zero replied-lead violations) is something an operator cannot pass while the product is broken. The pre-launch runbook with captured artefacts is something an operator cannot fake. The Playwright suite running against real integrations is something a feature regression cannot hide from. The three-layer construction (automation + manual + real-world) is the intentional defense against the failure mode of the prior three.

### What the operator does NOT do during this PRD

Argos memory and skills changes, magikshot SEO work, magiktext build work, training-load tracking, Notion daily pages — all continue on their own cadence. This PRD is bounded to magiklead's path to public launch.
