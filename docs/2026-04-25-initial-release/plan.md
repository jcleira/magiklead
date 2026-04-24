## Initial Release — Implementation Plan

**Date:** 2026-04-25
**Depends on:** `../2026-04-21-lead-database-architecture/plan.md` (T01–T16 shipped)
**Goal:** Take MagikLead from "plumbing exists" to "first paying customer can sign up, search the canonical lead DB, run a campaign, and we get paid". This is the v1 release.

---

## Status of prior work

The lead-database-architecture plan landed all 16 tasks (canonical schema, ingest sources, entity resolution, search API, admin panel, GDPR erasure). The pieces below are what stand between that and a public launch.

Known gaps carried in from that work:
- Clerk JWT verification is still a placeholder on both BE (`internal/middleware/auth.go`) and FE (`frontend/src/hooks/use-api.ts`, `frontend/src/lib/admin-api.ts`) — every protected route trusts whatever string lands in the `Authorization: Bearer …` header.
- The legacy `leads` table has not been dropped; T13 added `tenant_leads` and the bridge column on `campaign_leads`, but several handlers (`internal/handler/leads.go`, the old `worker/discover.go` path) still write to `leads` directly.
- The customer-facing `/leads` page renders the old per-tenant `leads` rows; it has not been switched to the `POST /api/v1/leads/search` API shipped in T12.
- The canonical DB is empty in production — the ingest CLI (`cmd/ingest`) exists, sources work in tests, but no batch has ever been run against the real APIs.
- Privacy `POST /api/v1/privacy/erasure` is publicly callable with no identity verification (signed link / email confirmation TBD).

---

## Executor Notes

Phases are ordered by dependency, but tasks within a phase usually parallelize. **Phase 1 (Auth) blocks every other phase** — without real Clerk verification, none of the protected endpoints are safe to expose to the internet.

Each task lists a "Done when" so the executor can self-check. When a "Done when" condition is verified, mark the task complete in this file and move on; don't open follow-up tasks unless the verification turned up a real defect.

---

## Phase 1: Real authentication (blocks everything)

### T01: Verify Clerk JWTs on the backend

**Files:**
- `backend/internal/middleware/auth.go` (replace placeholder)
- `backend/go.mod` (add `github.com/clerk/clerk-sdk-go/v2` or equivalent)
- `backend/.env.example` (document `CLERK_SECRET_KEY`, `CLERK_PUBLISHABLE_KEY`, `CLERK_WEBHOOK_SECRET`)

**What:**
- Use the Clerk Go SDK to verify the JWT, extract `sub` (clerk user id) + email.
- Stash `clerk_id` (existing `UserClerkIDKey`) AND email in context so admin middleware can drop the second DB lookup.

**Done when:** A request with an invalid/expired token returns 401 with no DB query; valid token populates context end-to-end.

### T02: Wire Clerk on the frontend fetch helpers

**Files:**
- `frontend/src/hooks/use-api.ts`
- `frontend/src/lib/admin-api.ts`
- `frontend/src/middleware.ts` (replace stub with `clerkMiddleware()`)

**What:**
- Replace `const token = "dev-token"` with `await getToken()` from `@clerk/nextjs`.
- Mount `<ClerkProvider>` in `app/layout.tsx`.
- Update `middleware.ts` to use `clerkMiddleware` + `createRouteMatcher` for the protected paths (note Next 16 calls this file `middleware.ts` still; rename to `proxy.ts` is a separate codemod, see version-16 docs).

**Done when:** `/dashboard` redirects unauthenticated users to Clerk sign-in, and authenticated requests carry a real JWT that the BE accepts.

### T03: Clerk webhook → users + tenants

**Files:**
- `backend/internal/handler/clerk.go` (already exists — verify the create path)
- `backend/queries/users.sql` / `tenants.sql`

**What:**
- On `user.created` event: insert into `users`, create a default tenant, link via `user_tenants`.
- On `user.updated`: keep email/name in sync.
- Verify `CLERK_WEBHOOK_SECRET` signature.

**Done when:** A new Clerk signup produces matching `users` + `tenants` + `user_tenants` rows automatically.

---

## Phase 2: Customer search experience

### T04: Replace `/leads` page with canonical search

**Files:**
- `frontend/src/app/(app)/leads/page.tsx` (rewrite)
- `frontend/src/lib/leads-api.ts` (new — wrap `POST /api/v1/leads/search`)

**What:**
- Title fuzzy filter, "with verified email" toggle, pagination.
- Result rows show person + current title + org + email status.
- Action to "save lead" → `POST /api/v1/leads` (or new `/tenant_leads/add` endpoint, see T05).

**Done when:** A logged-in user searches for "VP Sales" and sees real persons from the canonical DB (not the legacy `leads` table).

### T05: Saved-leads endpoint over `tenant_leads`

**Files:**
- `backend/internal/handler/tenant_leads.go` (new)
- `backend/queries/tenant_leads.sql` (already has Add/Remove/List/UpdateStatus — wire them up)
- `backend/cmd/api/main.go` (mount routes)

**What:**
- `POST /api/v1/tenant_leads` — save person to the tenant's list
- `GET /api/v1/tenant_leads` — list saved
- `PATCH /api/v1/tenant_leads/:person_id` — status / notes
- `DELETE /api/v1/tenant_leads/:person_id`

**Done when:** "save lead" from T04 persists to `tenant_leads` and shows up in a "Saved" tab.

### T06: Cut campaigns over to `person_id`

**Files:**
- `backend/internal/worker/discover.go`
- `backend/internal/worker/sender.go`
- `backend/internal/handler/campaigns.go`

**What:**
- Replace `campaign_leads.lead_id` writes with `campaign_leads.person_id` (column already exists per migration 016).
- Sender pulls email via `persons → emails` not `leads.email`.
- Audit any other `repository.Lead*` callers.

**Done when:** A new campaign created post-cutover writes only `person_id`; running campaigns continue to deliver via the new path.

### T07: Drop the legacy `leads` table

**Files:**
- `backend/migrations/017_drop_leads.up.sql` (new)
- `backend/migrations/017_drop_leads.down.sql` (new — recreate empty for rollback)
- `backend/queries/leads.sql` (delete)
- Anything still importing `repository.Lead*` (delete)

**Pre-req:** T06 verified in production for ≥ 1 week; double-check nothing else writes to `leads`.

**Done when:** Migration applied, `go build` clean, no runtime errors over a soak window.

---

## Phase 3: Real ingestion (populate the DB)

### T08: SEC EDGAR first batch

**Files:**
- Operational: `make ingest-edgar` or one-off `go run ./cmd/ingest sec-edgar --since 2024-01-01`

**What:**
- Provision the MinIO/S3 bucket in production (`docs/.../research.md` §"Storage" lists the env vars).
- Run a backfill from 2024-01-01 forward. Expected ~500K exec-company-title tuples.
- Watch `evidence.conflict_flag = true` rate; if > 10% of new records hit the queue, tune resolver thresholds before adding the next source.

**Done when:** ≥ 100K canonical persons with at least one current employment.

### T09: Wikidata + CrunchBase

**Files:** operational

**What:**
- Wikidata SPARQL with the truthy-claims query from T06 of the prior plan (rate-limit-aware retries already in `sources/wikidata.go`).
- CrunchBase: download the daily CSV dump, parse via `sources/crunchbase.go`. Verify the test fixture path works in production (gotcha: `/tmp/crunchbase-fixture/` only exists in dev containers).

**Done when:** Total canonical persons ≥ 500K; cross-source dedup rate (per `evidence` joins) > 20% on new ingests.

### T10: Email verification batch

**Files:** operational + `backend/cmd/verify-emails/main.go`

**What:**
- Configure SMTP probe sender (warm-up needed if from a fresh domain — see deliverability docs).
- Run nightly cron over `emails WHERE verified_at IS NULL`.
- Honor the `is_catchall` short-circuit per T10 in the prior plan.

**Done when:** Nightly job verifies new emails without bouncing the sender domain (catch-all detection prevents the bounce problem from prior incidents).

---

## Phase 4: Send pipeline E2E

### T11: Gmail OAuth send works against a live account

**Files:**
- `backend/internal/gmail/sender.go`
- `backend/internal/worker/sender.go`

**What:** Connect a real Gmail account via the existing OAuth flow, run a test campaign, verify the email arrives + tracking pixel fires + reply detection works.

**Done when:** A test send to a personal address arrives, opens are recorded in `email_events`, and a reply marks the campaign_lead as `replied`.

### T12: SMTP send path

**Files:**
- `backend/internal/handler/email_accounts.go`
- `backend/internal/worker/sender.go`

**What:** Same as T11 but via a generic SMTP account (Mailgun / Postmark / Sendgrid etc). Verify daily-quota enforcement actually trips at the configured limit.

**Done when:** SMTP send arrives + open/reply tracking parity with Gmail.

---

## Phase 5: Payments

### T13: Stripe checkout + webhook end-to-end

**Files:**
- `backend/internal/handler/billing.go`
- `frontend/src/app/(app)/settings/page.tsx` (or wherever billing UI lives)

**What:** Walk a test customer from "free" → checkout session → paid plan via webhook. Verify `subscriptions` row is updated with `current_period_end` + plan name.

**Done when:** Stripe test-mode purchase reflects in the user's plan within seconds of webhook delivery.

### T14: Plan-limit enforcement

**Files:**
- `backend/internal/handler/leads_search.go` (gate by `subscriptions.leads_limit`)
- `backend/internal/handler/sequences.go` (gate by `sequences_limit`)
- `backend/pkg/errors/errors.go` (`ErrQuotaExceeded` already exists)

**What:** When a tenant's `leads_used` >= `leads_limit`, return `429 quota_exceeded`. Increment `leads_used` per saved lead (T05).

**Done when:** Free-plan account bumps the wall after the configured limit; paid plan keeps going.

---

## Phase 6: Privacy + compliance

### T15: Erasure confirmation flow

**Files:**
- `backend/internal/handler/privacy.go` (split into request + confirm)
- `backend/migrations/018_privacy_requests.up.sql` (new — store pending tokens)
- `backend/internal/email/` (new — tiny transactional sender if Gmail/SMTP code isn't reusable)
- `frontend/src/app/(marketing)/privacy/erasure/page.tsx` (new — public form + confirmation landing)

**What:**
- `POST /api/v1/privacy/erasure/request` accepts identifiers, generates a signed token, emails it to the address in the request, returns 202.
- `POST /api/v1/privacy/erasure/confirm?token=…` runs the existing erasure logic.
- Token expires in 24h.

**Done when:** Submitting the form sends a confirmation email; clicking the link deletes the record. Without the click, nothing happens.

### T16: Privacy policy + ToS wiring

**Files:**
- `frontend/src/app/(marketing)/privacy/page.tsx` (already exists — fact-check)
- `frontend/src/app/(marketing)/terms/page.tsx` (already exists — fact-check)

**What:** Make sure the published policy actually matches what the system does (canonical DB, blocklist, GDPR endpoint, retention).

**Done when:** A lawyer-friendly read finds no contradiction between the policy and observable system behavior.

---

## Phase 7: Deploy + monitor

### T17: Backend deploy

**Files:**
- `backend/Dockerfile` (already exists — verify multi-stage)
- `backend/Dockerfile.worker`
- Deployment config (Hetzner Cloud + Caddy is already partially set up per `Caddyfile`; finish it)

**What:**
- Provision: PostgreSQL (Hetzner managed or self-hosted), Redis, MinIO/S3, the API + worker containers.
- Caddy in front for TLS.
- Env secrets via `.env.production` (or whatever secret store).

**Done when:** `https://api.magiklead.com/health` returns `ok` over TLS.

### T18: Frontend deploy

**What:**
- Vercel for the marketing + app, with `NEXT_PUBLIC_API_URL` pointed at T17.
- Custom domain + SSL via Vercel.

**Done when:** `https://magiklead.com` serves the marketing site; `https://app.magiklead.com` (or a path) serves the authenticated app.

### T19: Error tracking + uptime

**What:**
- Sentry (or equivalent) on both BE + FE.
- Uptime check on `/health` + a synthetic that hits `/api/v1/leads/search` with a known query.

**Done when:** A deliberately thrown error in dev shows up in the dashboard within seconds; uptime alerts fire when the API is down.

---

## Phase 8: Onboarding + launch

### T20: Onboarding flow E2E

**Files:**
- `frontend/src/app/(app)/onboarding/page.tsx` (currently uses `mockProfile` / `mockPlays` — wire to real `/api/v1/websites/analyze` + `/api/v1/plays/generate`)

**What:** New user signs up → onboarding asks for their website → AI analyzes → suggests plays → user picks → first campaign is created.

**Done when:** A fresh signup can go from landing page to first campaign without seeing any mock data.

### T21: Launch checklist

- DNS records (MX, SPF, DKIM, DMARC) for the sender domain
- Stripe live mode swap
- Clerk production instance swap
- Pricing page reflects real plan names + Stripe price IDs
- Status page (statuspage.io / Instatus / homemade)
- Refund policy + support email reachable

**Done when:** All boxes ticked. Then launch.

---

## What's intentionally NOT in this plan

- **More ingest sources beyond Wikidata + CrunchBase + EDGAR.** Common Crawl team-page extraction (T09 of the prior plan) has code but is operationally heavy; defer until v1 customers tell us coverage is the bottleneck.
- **A "find duplicates" UX in the admin panel.** T15 of the prior plan exposed manual merge but no candidate suggestion. Add only if conflict volume justifies it.
- **Mobile app.** Not needed for v1.
- **Multi-tenant admin** (separate admin per tenant). The current `ADMIN_EMAILS` env var is staff-only and that's fine for v1.

---

## Open questions for jcleira

1. **Hosting target** — Hetzner Cloud + self-managed Postgres, or fully managed (Supabase / Neon / Render)? Affects T17.
2. **Sender domain** — magiklead.com itself, or a separate `mail.magiklead.com` for deliverability isolation?
3. **Pricing tiers** — research.md mentioned $49/mo entry; are the higher tiers locked in? Affects T13/T14.
4. **Launch audience** — soft launch to existing waitlist, or wide-open public? Affects T19/T21 risk tolerance.
