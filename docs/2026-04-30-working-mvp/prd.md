# Working MVP — PRD

**Date:** 2026-04-30
**Supersedes:** `../2026-04-29-zero-friction-smoke/plan.md` (folds its scope in as one of several work items).
**Depends on:** `../2026-04-25-initial-release/plan.md` (T01–T20 landed in the working tree, not yet committed).

---

## Problem Statement

MagikLead has a complete codebase but no working product. The dev workstation can't sign a new user up without a terminal command (`cmd/dev-create-user`), the canonical lead database is empty after migrations, the ingest pipeline has never been pointed at real APIs, and there is nothing deployed anywhere — no public URL, no DNS, no monitoring, no transactional email path that survives outside the devpod's MailHog. As a result, "the product" exists only as code in a working tree. It cannot be validated end-to-end, demoed, or used by anyone, including the operator. That is a half-built thing, and half-built things rot.

The operator wants a working site: real domain, real lead data, real deploy, error-visible — running end-to-end on infrastructure they control, so they can use it themselves and decide whether the product is ready to advertise.

The two things this PRD does *not* try to solve are deliberate: production Clerk and Stripe instances will be swapped in only after the operator has used the site themselves and is satisfied the product works as advertised, and the public launch (active customer acquisition, audience-building) is its own separate effort.

## Solution

Take MagikLead from "code" to "running site the operator can use." Land the lazy-tenant-bootstrap that makes local sign-up zero-friction, ship the canonical seed fixture so a fresh devpod returns search results immediately, run the ingest pipeline against the three free real-data sources (EDGAR, Wikidata, TeamPages) so the canonical database has tens of thousands of real exec rows in production, deploy the api + worker + frontend onto a single Hetzner VPS behind a real domain with SSL, wire up Sentry for error tracking, configure a transactional email provider for system-originated mail (erasure confirmations, etc.), and set up SPF/DKIM/DMARC for the sender domain.

After this PRD, the operator can sign up at `https://magiklead.com` (or whichever domain ends up registered), walk the entire smoke path against real lead data, and any errors are reported in Sentry. The dev Clerk and dev Stripe instances continue to back the site during this validation window. When the operator is satisfied, a small follow-up PRD swaps the keys, points DNS at the production identity, and announces the launch.

## User Stories

### Local development

1. As a developer, I want a fresh sign-up at `mvp.magiklead.localhost` to materialise my user, tenant, role, and free subscription on the first protected request, so that I never need to run a terminal command to bootstrap myself.
2. As a developer, I want a fresh `devpods up` to land on a database that already contains the canonical fixture (5 organisations + 20 persons), so that the `/leads` search returns rows immediately on first visit.
3. As a developer, I want the legacy `dev-create-user` CLI deleted, so that the only path to user creation is the production-shaped one.
4. As a developer, I want the existing `user.created` Clerk webhook to keep working, so that production setups (where the webhook is reachable) get an instant bootstrap rather than a lazy one.
5. As a developer, I want the existing `user.updated` webhook to keep syncing email and name changes, so that profile updates from Clerk reach the database without me touching anything.
6. As a developer, I want bootstrap failures to fail loudly (HTTP 5xx with a logged reason) rather than silently leaving partial rows, so that a half-bootstrapped account is impossible.
7. As a developer, I want the bootstrap path to be safe under concurrent first-requests, so that two simultaneous requests cannot create two tenants or two users for the same Clerk identity.
8. As a developer, I want a missing JWT `email` claim to surface as a clear configuration error at request time, so that I notice a misconfigured Clerk JWT template the first time I hit it.

### Real lead data

9. As an operator, I want the EDGAR ingest source to run against the real SEC quarterly bulk dumps, so that the canonical database contains every executive officer of every US public company who filed a Form 3/4/5 in the last several quarters.
10. As an operator, I want the Wikidata ingest source to run against the live SPARQL endpoint, so that notable global CEOs (with Wikidata entries and the right `position held` claim) appear in the canonical database, deduped against EDGAR via SEC CIK.
11. As an operator, I want the TeamPages ingest source to run against a curated seed list of company domains, so that private companies whose `/team` or `/leadership` pages list executives are represented in the canonical database.
12. As an operator, I want the email-verification batch to run after ingest so that emails on imported persons carry a real verification status, so that the `/leads` "with verified email" filter has signal.
13. As an operator, I want the post-ingest canonical database to be backed up on a schedule, so that a Postgres failure does not destroy the ingested corpus.
14. As an operator, I do not want to run the CrunchBase ingest in this round, so that the v1 cost is zero for data and we can re-evaluate CrunchBase post-launch.

### Production deployment

15. As an operator, I want the api, worker, frontend, Postgres, Redis, and S3-compatible object storage to run on a single Hetzner VPS via docker-compose, so that the production layout is the smallest possible step from the dev devpod layout.
16. As an operator, I want the production app reachable at the project's real domain over HTTPS with a valid certificate auto-renewed, so that I never have to think about SSL.
17. As an operator, I want the frontend served from the same VPS rather than from Vercel, so that I am off the platform I am migrating away from.
18. As an operator, I want production secrets (Clerk dev keys, Anthropic, Stripe dev keys, Sentry DSN, Resend or equivalent SMTP credentials, ingest source environment variables) loaded from a single deployable env file the operator owns, so that nothing about my deploy depends on devpods-specific scaffolding.
19. As an operator, I want a one-command deploy (`make deploy` / `./deploy.sh` / equivalent) that pulls the latest code, rebuilds containers, runs migrations, and rolls the running services, so that updates are routine.
20. As an operator, I want the production stack to be reproducible from version-controlled config, so that re-creating the box from scratch is a known procedure rather than tribal knowledge.
21. As an operator, I want the production database to be backed up nightly to off-host storage (Hetzner Storage Box or equivalent), so that a VPS loss does not lose data.
22. As an operator, I want errors (panics, 5xx, unhandled promise rejections on the frontend) reported to Sentry with the user's Clerk ID and tenant attached, so that I see what is breaking without tailing logs.
23. As an operator, I want uptime monitored externally (Better Stack, UptimeRobot, or equivalent) so that a VPS reboot or process crash pages me.
24. As an operator, I want the system transactional mailer (used today only for erasure confirmations) to send through a real provider in production, so that erasure links actually arrive.

### Domain and email authentication

25. As an operator, I want the project domain registered and pointing at the Hetzner VPS, so that the site has a real URL.
26. As an operator, I want SPF, DKIM, and DMARC configured for the system sender domain, so that erasure confirmation emails do not land in spam.
27. As an operator, I want the customer-side outbound path (campaigns sent via the customer's Gmail OAuth or SMTP) untouched by this PRD, so that customer deliverability remains the customer's responsibility.

### After execution

28. As the operator, I want to walk the entire smoke path on the deployed site (sign up → onboarding → search → save → erasure-confirm → admin gates) and find no broken steps, so that I can decide the product is fit for paying users.
29. As the operator, I want a deploy that is safe to leave running for days while I dogfood it, so that I am not paying ongoing attention to keep it alive.
30. As the operator, I want a clear list of what is *still* required for public launch (production Clerk swap, production Stripe swap, public DNS cutover, marketing) at the bottom of this PRD, so that I know exactly what the next plan covers.

## Implementation Decisions

### Tenant bootstrap (deep module)

A single internal package encapsulates the four-row provisioning sequence (`users` row, `tenants` row, `user_tenants` link, free `subscriptions` row). It exposes one function whose signature takes a connection pool, a Clerk identity, an email, and an optional display name, and returns the resulting tenant identifier. The function runs all four writes inside a single Postgres transaction; any failure aborts the whole sequence so partial state is impossible. Two callers consume this module: the lazy-bootstrap middleware on the protected request path, and the Clerk webhook handler on `user.created`. The webhook keeps existing because it is faster than lazy bootstrap when the public webhook URL is reachable.

User creation uses `INSERT ... ON CONFLICT (clerk_id) DO UPDATE` returning the row, which makes the path safe under concurrent first-requests and side-effect-doubles as the up-to-date sync for email and name. A row-level lock on the user row inside the transaction serialises the rest of the bootstrap so two simultaneous requests cannot create two tenants for the same user.

The bootstrap module returns a clear typed error when the JWT `email` claim is empty; the middleware surfaces that as HTTP 503 with a body that names the JWT-template configuration as the cause, so the developer recognises the misconfiguration on the first request rather than chasing a downstream NULL.

The default workspace name when no display name is available is the email's local part with a possessive suffix (e.g. `tim's Workspace`), matching the existing CLI's behaviour.

### Tenant-resolution middleware

The existing tenant middleware is rewritten in place. Its contract changes from "look up the user's first tenant or pass through" to "guarantee a tenant identifier in the request context or reject the request." On a missing user, it calls the bootstrap module and continues. On any error from bootstrap, it returns 5xx and logs. The function name is updated to reflect the new contract.

### Clerk webhook handler

The `user.created` branch is refactored to delegate to the bootstrap module. The `user.updated` branch is left in place but is now redundant for user-row sync (the upsert in the bootstrap module already keeps email/name fresh on every protected request). It stays because (a) Clerk keeps firing it whether we use it or not, (b) production has historically relied on it, and (c) the cost of leaving it running is one row update per Clerk event.

### Devpods seed snapshot

The plan does *not* add a runtime probe-and-seed step in the api entrypoint. Instead, this PRD uses the seed pipeline that already exists in `devpod/devpod.yml` and `devpod/seeds/regenerate.sh`: the executor runs `devpods seed regenerate` once, which produces `db.sql.gz` and `manifest.json` in the seeds directory, and commits both artefacts to the repository. From that point on, `devpods up` automatically replays the snapshot on a fresh database, the canonical fixture appears for free, and there is nothing extra in the api startup path. The manifest's migration-hash check makes seed-vs-schema drift detectable.

### Legacy CLI removal

`backend/cmd/dev-create-user` is deleted. The lazy-bootstrap path supersedes it. Documentation references to the CLI are removed.

### Real ingest run

The ingest pipeline (`cmd/ingest`) is run end-to-end against three sources: SEC EDGAR (free quarterly bulk TSV), Wikidata (free public SPARQL), and TeamPages (free, scrapes /team /about /leadership pages from a curated seed list of company domains, with Claude-driven extraction). The CrunchBase source is intentionally skipped — its CSV path expects user-supplied data and an acquisition cost is not justified for v1.

Each source is run with a sensible since-date or seed list, the resolver runs across the imports to produce canonical persons + organisations, and the email-verification batch runs against the resulting emails. The seed list for TeamPages is committed to the repository so future runs are reproducible.

The ingest is orchestrated as a one-shot operational task on the production box (or, if cleaner, a local run that dumps and restores into production). The result is a canonical database with real data, on which the smoke path returns meaningful search results.

### Hetzner deploy

Single VPS (CCX23 or similar, 4 vCPU / 16 GB RAM ballpark — start small, resize is one-click on Hetzner). The production layout is a docker-compose file mirroring the devpod layout: api + worker + frontend + Postgres + Redis + MinIO (or Hetzner Object Storage S3-compat endpoint, if the operator prefers a managed option), all behind Caddy as the reverse proxy with auto-LetsEncrypt. Caddy gets the operator off the SSL-management treadmill — its config is a few lines to map domains to upstream containers. The compose stack runs as a systemd unit so the box self-recovers from reboot.

A `deploy/` directory at repo root holds: `docker-compose.prod.yml`, `Caddyfile`, an `.env.production.example` template (matching the devpod env files but flat), and a deploy script that pulls latest, rebuilds, runs migrations, and rolls services with no-downtime restarts (compose's recreate-on-change with healthchecks suffices at this scale).

Postgres backups: nightly `pg_dump` to a Hetzner Storage Box mounted on the VPS, with a 14-day retention. Object-storage backups: lifecycle copy to a second bucket if using Hetzner Object Storage, or a nightly `mc mirror` if using MinIO.

### Production transactional email

The system mailer (used for erasure confirmation today; will be used for any future system-originated mail) sends through Resend in production. Resend is the cheapest path to clean DKIM and decent deliverability: a sub-$20/mo plan covers the volume an early MVP would generate, and DKIM is a single Cloudflare/registrar TXT record that Resend's dashboard hands you.

The existing `internal/email` package already abstracts the SMTP send; in production it points at Resend's SMTP relay rather than the devpod's MailHog. No code change required beyond environment configuration.

### Sender domain authentication

DNS records on the project domain include: SPF (`v=spf1 include:_spf.resend.com -all` or equivalent for the chosen provider), DKIM (the TXT record from Resend's dashboard), and DMARC (start with `p=none rua=mailto:dmarc@...` to monitor before enforcing). These authenticate the system transactional sender. Customer outbound (campaign sends through customer Gmail OAuth or customer SMTP) is unaffected — that authentication is the customer's responsibility, by design.

### Error tracking

Sentry on both backend (Go SDK) and frontend (Next.js SDK). The backend reporter pulls the Clerk user ID and tenant ID from the request context onto every error event so issues are attributable. The frontend reporter pulls the Clerk user ID from `useAuth`. Free tier covers expected MVP volume.

### Uptime monitoring

External pinger (Better Stack, UptimeRobot, or equivalent) hits `/health` every 60 seconds and pages the operator on three consecutive failures. Both services have free tiers that cover one or two endpoints.

### Domain

The project's public domain is registered (TLD-of-the-operator's-choice — to be confirmed at execution time, default assumption `magiklead.com`) and its A record points at the Hetzner VPS. Caddy on the VPS serves both apex (frontend) and `api.` subdomain (backend) with auto-LetsEncrypt.

## Testing Decisions

A good test exercises external behaviour: input goes in, output comes out, side effects you can observe. Tests should not assert on internal call sequences or private types — those change under refactor and the test rots with them. The bootstrap module has a clear external boundary (a function with four arguments and one return value) and a clear set of side effects (four rows inserted within one transaction); that boundary is the test target.

### Bootstrap module

Three tests:

1. **Happy path.** Call with a fresh Clerk identity. Assert: returned tenant ID is non-zero, the four expected rows exist with the expected linkage, and the subscription is on the free plan with the right limits.
2. **Idempotent re-call.** Call once with a fresh identity, call again with the same identity. Assert: the second call returns the same tenant ID as the first, no extra rows are created in any of the four tables.
3. **Transaction rollback.** Inject a failure on the third write (e.g. a constructed constraint violation on `user_tenants`). Assert: zero new rows in any of the four tables, error returned.

These run against a real Postgres (the devpod's Postgres in CI/local, a tmp database in CI). Mocking the four queries individually would test the wiring rather than the behaviour; this PRD picks integration-with-real-DB instead because the transactional guarantees are the substantive thing under test.

### Tenant middleware

Two tests, mirroring the existing `admin_test.go` pattern (nil-or-stub handler, manipulated context, assertions on the response):

1. User exists, tenant exists. Assert: tenant ID lands in context, downstream handler runs, bootstrap is not called.
2. User does not exist. Assert: bootstrap is called once with the JWT identity, tenant ID lands in context, downstream handler runs.

The bootstrap function is invoked through a small interface so the middleware test substitutes a stub. This matches the existing prefer-no-DB-in-handler-tests pattern in the codebase.

### Smoke path (manual)

The deployed site walks the predecessor plan's T03 verification with the addition of "uses the real domain over HTTPS":

1. Sign up at `https://<domain>` via Clerk's hosted page using a real email.
2. Land in onboarding, paste a website URL, AI analyses, pick plays, first campaign created.
3. `/leads` → search "VP Sales" → real persons appear (from the EDGAR/Wikidata/TeamPages ingest) → save 2–3.
4. `/privacy/erasure` (different browser, no auth) → submit your email → click the confirmation link in the *real* inbox the operator owns (not MailHog).
5. If your email is in `ADMIN_EMAILS`, `/conflicts` and `/persons/<id>` are reachable.

Done when all five steps succeed end-to-end against production.

## Out of Scope

- **Production Clerk instance + key swap.** The site runs on dev Clerk during the operator's validation window. After validation, a follow-up PRD creates a production Clerk instance, configures its JWT template and webhook, swaps the keys, and updates DNS for any auth-domain CNAME Clerk requires. Same for the Stripe production instance and product/price IDs.
- **CrunchBase ingest.** The CrunchBase source code remains in the repo for future use but is not run in this PRD. Re-evaluate post-launch.
- **Public launch.** Active customer acquisition, marketing, and audience-building are their own effort. This PRD ends at "operator-validatable site running on real infrastructure."
- **Customer outbound deliverability (warmup, dedicated sender domain pools).** Customers send through their own Gmail or SMTP; their deliverability is their problem. The deliverability-check endpoint exists; nothing extra is wired in this PRD.
- **Multi-region or HA deploy.** Single VPS, one process per service. Resize or add HA after we know what the load looks like.
- **Web Application Firewall, advanced rate limiting, bot protection.** Caddy's defaults plus Cloudflare in front of the domain (free tier, optional) are enough until traffic warrants more.
- **Auditable change history for admin actions.** The admin gate exists; an audit log is a separate plan.

## Further Notes

### Sequencing

The PRD's items are not all the same shape. Roughly:

1. **Code work first** — bootstrap module + middleware rewrite + webhook refactor + dev-create-user delete + seed snapshot. Lands as one branch, reviewed, merged. The smoke path on the local devpod proves nothing is regressed.
2. **Real ingest run** — operational, runs locally against real APIs first to validate, then on the VPS once it exists. Produces a populated database. EDGAR's quarterly dump is a few GB; Wikidata's SPARQL pull is small; TeamPages depends entirely on the seed list size.
3. **Hetzner setup** — provision the VPS, install Docker + Caddy, copy the production compose file, configure DNS, get the apex serving HTTPS with the dev Clerk and Stripe keys.
4. **Migrate Postgres data** — `pg_dump` from the local environment with the populated database, restore on the VPS. This avoids re-running ingest in production.
5. **Wire monitoring** — Sentry SDKs, uptime pinger, log retention.
6. **Configure transactional email and DNS auth records** — Resend account, DKIM/SPF/DMARC TXT records.
7. **Smoke walk** — operator runs T03's five steps on the real domain. Anything broken gets fixed before the follow-up launch PRD opens.

### Ballpark cost

- Hetzner VPS: ~€20/mo for the suggested size.
- Hetzner Storage Box for backups: ~€4/mo for 1 TB.
- Domain: depends on TLD, ~€10–15/year.
- Resend: $0 on the free tier (3k emails/mo) until volume picks up.
- Sentry: $0 on the free tier.
- Uptime monitor: $0 on free tiers.
- CrunchBase: $0 (skipped).

Total: roughly €25/mo plus the one-time domain cost. Specifically not a five-figure-anything.

### What comes next

After this PRD and the operator's local validation, a small follow-up PRD covers:

- Create production Clerk instance, configure JWT template + webhook, generate keys.
- Create production Stripe instance, copy products + price IDs, generate keys.
- Swap dev keys → prod keys in the production env file.
- Cut DNS over to production identity (auth domain CNAMEs, etc.).
- Public launch checklist (status page, support inbox, terms/privacy review, the rest of the predecessor plan's T21).

That follow-up is small precisely because this PRD does the heavy lifting.

### Operator caveats

- The domain is not yet registered. The executor should ask the operator to register it (or do it themselves with a card on file) before configuring DNS. Default assumption in the PRD: `magiklead.com`. Override at execution time.
- The Hetzner VPS does not yet exist. The executor provisions it during the Hetzner-setup step. SSH key is needed up front.
- The operator's email address must be added to `ADMIN_EMAILS` on the production env so the admin gate works after deploy.
- The dev Clerk JWT template `magiklead-backend` must already be configured in the dev Clerk dashboard (see project CLAUDE.md). Without it, the lazy-bootstrap middleware returns 503 with a clear message — fix in the dashboard, retry.
