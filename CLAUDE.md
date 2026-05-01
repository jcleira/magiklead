# MagikLead

Outreach product. Go backend (`backend/`) + Next.js 16 frontend
(`frontend/`). The lead-database architecture and the initial-release
plan live under `docs/` (see `docs/2026-04-25-initial-release/plan.md`
for what's currently being delivered).

## Devpod

This project uses [devpods](https://github.com/jcleira/devpods) for
isolated per-worktree local development. Each worktree gets its own
Postgres / Redis / MinIO / MailHog / Stripe-CLI listener, addressed
at `<devpod-name>.magiklead.localhost` via a shared Traefik.

The devpod name is derived from the branch — `magiklead-mvp` becomes
`mvp`, so URLs land at `http://mvp.magiklead.localhost` (frontend),
`http://api-mvp.magiklead.localhost` (backend), and
`http://mail-mvp.magiklead.localhost` (MailHog UI for transactional
mail).

Migrations apply automatically on `devpods up` — the api service's
entrypoint runs `go run ./cmd/migrate` before handing control to air.

### Commands Claude may run

- `devpods up` — provision the devpod for the current worktree (idempotent)
- `devpods down` — destroy it (`--force` in non-interactive mode)
- `devpods status` — services, URLs, health (supports `--json`)
- `devpods logs [api|worker|frontend]` — tail logs
- `devpods psql` — psql shell into the devpod DB (user/db: `devpod`)
- `devpods redis-cli` — redis-cli into the devpod Redis
- `devpods curl <path>` — curl the devpod API with the right Host header
- `devpods exec <svc> <cmd>` — run a command inside a service container
- `devpods list` — all running devpods across projects
- `devpods seed` — reset DB + storage to the baseline seed snapshot
- `devpods seed regenerate` — rebuild the seed snapshot via
  `devpod/seeds/regenerate.sh` (runs `cmd/seed` then `pg_dump`)

### Loading the canonical fixture (for testing search/save/campaign)

The canonical `persons` graph is empty after migrations apply — the
real ingest pipeline is operational (Phase 3 of the plan). For local
testing the in-tree fixture loader inserts 5 organizations + 20
persons + verified emails:

```
devpods exec api go run ./cmd/seed
```

Idempotent: re-running clears prior fixtures by `(devpod-fixture)`
suffix and replants them. Doesn't touch any non-fixture rows.

### Info files

- `.devpod.env` (workspace root, gitignored) — current devpod name + URLs
- `devpod/devpod.yml` — declares modules and services
- `devpod/compose.yml` — project-specific service definitions
- `~/.config/devpods/magiklead/.env.{backend,frontend}` — user-owned
  secrets (Clerk, Anthropic, Stripe, Gmail OAuth). Not committed.

### Required secrets

`~/.config/devpods/magiklead/.env.backend` (used by api + worker):

- `CLERK_SECRET_KEY` — required at api startup, fails fast otherwise
- `CLERK_PUBLISHABLE_KEY` — for completeness; backend doesn't read it
- `CLERK_WEBHOOK_SECRET` — Svix signature for `user.created` /
  `user.updated` events; without it the webhook returns 503
- `ANTHROPIC_API_KEY` — needed for `/websites/analyze` and
  `/plays/generate` (the onboarding flow)
- `ADMIN_EMAILS` — comma-separated list of admin emails. Match the
  Clerk JWT `email` claim — case-insensitive
- `STRIPE_SECRET_KEY` (optional) — only if you want to exercise T13
- `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` /
  `GOOGLE_REDIRECT_URI` (optional) — only for Gmail OAuth send tests

`~/.config/devpods/magiklead/.env.frontend`:

- `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY`
- `CLERK_SECRET_KEY` (Clerk's Next.js middleware reads it server-side)

Devpods generates `DATABASE_URL`, `REDIS_URL`, `S3_*`, `APP_URL`,
`FRONTEND_URL`, and (when the stripe module is enabled)
`STRIPE_WEBHOOK_SECRET` automatically — do NOT set those in
`.env.backend`. The api compose entrypoint reads
`/shared/stripe-webhook-secret` (written by the `stripe-listen`
container) and exports `STRIPE_WEBHOOK_SECRET` before starting air.

### Clerk dashboard prereqs

1. Create a JWT template named **`magiklead-backend`** with body
   `{"email": "{{user.primary_email_address}}"}`. Without it the
   backend's admin gate returns 403 "Missing email claim".
2. Add a webhook endpoint with events `user.created` + `user.updated`
   pointing at `http://api-<devpod>.magiklead.localhost/api/v1/webhooks/clerk`.
   Clerk can't reach `.localhost` directly — for local testing, expose
   it via `cloudflared tunnel run` or `ngrok http 80` and use the
   tunnel URL.
