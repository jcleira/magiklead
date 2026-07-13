# MagikLead

Multi-tenant cold-outreach SaaS: describe your ideal buyer, let the
product find prospects in a shared canonical lead database, generate a
message sequence with Claude, and run the campaign from your own
accounts over one of two send rails.

- **Email** — sends from the customer's own Gmail (OAuth). Reply and
  bounce detection, suppression lists, per-tenant quotas, Stripe
  billing, and RFC 8058 one-click unsubscribe.
- **LinkedIn** — sends from the customer's own LinkedIn account
  through [Unipile](https://www.unipile.com): paced connection
  invites, first DM on acceptance, follow-ups that halt on reply, a
  multi-week warmup ramp, an acceptance-rate circuit breaker, invite
  withdrawal, and restriction/disconnect handling. LinkedIn caps
  invites at roughly 100–150 per account per week, so capacity — and
  pricing — is metered per connected account, not per lead.

The LinkedIn rail exists because email data is structurally expensive
(≈$0.29 per lead-with-email via People Data Labs), while a LinkedIn
profile URL *is* the address — nothing to find, verify, or buy per
lead. Full rationale in
[docs/2026-06-09-linkedin-only-outreach/prd.md](docs/2026-06-09-linkedin-only-outreach/prd.md).

## Repository layout

| Path | Contents |
| --- | --- |
| `backend/` | Go 1.26 services — chi HTTP API, asynq (Redis) workers, pgx + sqlc on Postgres, SQL migrations. |
| `frontend/` | Next.js 16 App Router — React 19, Tailwind 4, shadcn/ui, Clerk auth. |
| `tests/e2e/` | Playwright suite: 20 flows from signup through LinkedIn campaign metrics. |
| `devpod/` | Per-worktree local dev environment ([devpods](https://github.com/jcleira/devpods)) — service definitions + seed snapshot. |
| `docs/` | Dated phase folders (PRD + issue tracker each) — the project's chronological record. |
| `scripts/deploy.sh`, `docker-compose.prod.yml`, `Caddyfile` | Production deploy to a Hetzner VPS. |
| `Makefile`, `docker-compose.yml` | Bare-metal dev fallback (raw Postgres/Redis/MinIO plus `go run` / `pnpm dev`). |

## Backend

Entry points under `backend/cmd`:

| Command | Purpose |
| --- | --- |
| `api` | HTTP API — Clerk-authenticated `/api/v1/*`, public webhooks (Clerk, Stripe, Unipile), the Gmail OAuth callback, and one-click unsubscribe. |
| `worker` | asynq consumers — campaign scheduling and sending on both rails, Gmail reply/bounce polling, LinkedIn pacing, warmup, and reconcile polling. |
| `migrate` | Applies `backend/migrations`. |
| `seed` | Plants the devpod fixture lead graph (idempotent, rows suffixed `(devpod-fixture)`). |
| `ingest` | One ingestion pass against a named source: Crunchbase CSV, SEC EDGAR, Wikidata, team-pages scraper, or `toy`. |
| `verify-emails` | SMTP verification over unverified `emails` rows. Never invents addresses — pattern-guessing is banned. |

The domain core is a **canonical person graph** (`internal/leads`,
design in
[docs/2026-04-21-lead-database-architecture](docs/2026-04-21-lead-database-architecture)):
`organizations`, `persons`, their identifiers (email address, LinkedIn
URL) and employments, shared across tenants and filled by ingestion,
PDL search results, and email verification. Tenant-side leads
reference canonical persons, so data bought or scraped once is reused
by every tenant. Email-channel search falls back to People Data Labs
when the canonical graph runs dry; LinkedIn-channel search returns
persons keyed by `linkedin_url`, no email required.

Other notable modules: `internal/ai` (Claude-powered website analysis
and play/sequence generation for onboarding), `internal/gmail` (OAuth
connect, send, polling), `internal/linkedin` (Unipile client, webhook
handling, pacing), `internal/suppression`, `internal/bootstrap` (lazy
tenant bootstrap on first authenticated request), and an admin surface
for canonical-graph conflict review and person merge, gated by
`ADMIN_EMAILS`.

## Frontend

Route groups under `frontend/src/app`:

- `(marketing)` — landing, pricing, terms / privacy / refund, and
  competitor comparison pages.
- `(app)` — onboarding (analyze website → generate plays), dashboard,
  lead search and save, campaign authoring with sequence editing and
  metrics, deliverability, and settings (Gmail + LinkedIn connect,
  billing, account).
- `(admin)` — conflict review and person merge for the canonical
  graph.

## Local development

The supported path is [devpods](https://github.com/jcleira/devpods):
an isolated per-worktree stack (Postgres, Redis, MinIO, MailHog,
Stripe CLI) behind a shared Traefik. **[CLAUDE.md](CLAUDE.md) is the
full runbook** — required secrets in
`~/.config/devpods/magiklead/.env.{backend,frontend}`, Clerk dashboard
prerequisites, and known operational gaps. Once secrets are in place:

```sh
devpods up                          # provision; migrations run automatically
devpods exec api go run ./cmd/seed  # plant the fixture lead graph
```

Then open `https://mvp.magiklead.localhost` (branch `magiklead-mvp` →
devpod name `mvp`; https comes from a local mkcert + Traefik layer,
see CLAUDE.md).

`make dev` is a minimal non-devpod alternative: raw compose infra
(Postgres `:5555`, Redis `:6380`, MinIO `:9000`) with the API on
`:8040` and the frontend on `:3040`.

## Tests

```sh
devpods exec api go test ./...             # backend, inside the devpod (DB available)
cd tests/e2e && pnpm install && pnpm e2e   # Playwright, against a running stack
```

The e2e suite reads `E2E_FRONTEND_URL` (default
`http://mvp.magiklead.localhost`) and talks to the stack directly — it
has no devpods CLI dependency. CI
([.github/workflows/e2e.yml](.github/workflows/e2e.yml)) walks the
same flows on every backend/frontend PR and nightly, standing the
stack up natively on the runner; specs that need real third-party
credentials (Gmail Workspace, Stripe live, PDL) skip when the secrets
are absent.

## Deployment

```sh
./scripts/deploy.sh <server-ip>
```

Rsyncs the tree to `/opt/magiklead` on a Hetzner VPS, runs migrations,
and brings up `docker-compose.prod.yml`: `api` (:8080), `worker`,
`redis`, and Caddy terminating TLS for `api.magiklead.com`. Postgres
is not part of the prod compose — `DATABASE_URL` comes from the
server's `.env`. The frontend is deployed separately.

## Docs

Each dated folder under `docs/` is a phase with a `prd.md` and an
`issues/` tracker; read them in order for the full history:

| Folder | Phase |
| --- | --- |
| `2026-04-10-research`, `2026-04-10-plan-magiklead` | Original product research and plan. |
| `2026-04-13-lead-discovery-engine` | Lead-discovery engine design. |
| `2026-04-21-lead-database-architecture` | Canonical person-graph architecture and ingest roadmap. |
| `2026-04-25-initial-release` | v1 implementation plan (T01–T20). |
| `2026-04-29-zero-friction-smoke`, `2026-04-30-working-mvp` | Smoke-walk fixes; 12-slice hardening into a working MVP. |
| `2026-05-07-real-product-release` | Real-data release: Gmail send, reply/bounce polling, unsubscribe, PDL search, metrics, billing. |
| `2026-06-09-linkedin-only-outreach` | The LinkedIn rail — PRD + 9 slices, all closed. |
| `2026-06-25-finish-unipile-integration` | Rail validated against real Unipile payloads — 9 slices, all closed. |
| `2026-07-09-devpod-https-auth-handoff.md` | Devpod https/auth debugging state capture. |
| `design-tokens.md` | UI design tokens. |
