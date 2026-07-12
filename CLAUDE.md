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
testing the in-tree fixture loader inserts both channels' prospects:
5 organizations + 20 persons with verified emails (email prospects),
plus 2 organizations + 6 persons with `linkedin_url` identifiers and
no email (LinkedIn prospects — what the LinkedIn-channel search
returns):

```
devpods exec api go run ./cmd/seed
```

Idempotent: re-running clears prior fixtures by `(devpod-fixture)`
suffix (cascade drops their identifiers + employments) and replants
them. Doesn't touch any non-fixture rows. `devpods seed regenerate`
bakes the same data into the local snapshot.

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
  `GOOGLE_REDIRECT_URI` (optional) — only for Gmail OAuth send tests.
  `GOOGLE_REDIRECT_URI` MUST include the `/v1` prefix
  (`…/api/v1/gmail/callback`) and match a redirect URI registered on
  the Google OAuth client exactly; a value without `/v1` 404s
- `GMAIL_OAUTH_STATE_SECRET` — required by the api when
  `GOOGLE_CLIENT_ID` is set. HS256 key that signs the OAuth `state`
  param carrying the tenant + connecting user across the Google consent
  redirect; the `/api/v1/gmail/callback` route is public (Google
  arrives with no Clerk session) and trusts only this signed state. Any
  opaque 32+ byte string; keep it distinct from
  `UNSUBSCRIBE_SIGNING_SECRET`
- `PDL_API_KEY` (optional) — People Data Labs key for the lead-search
  PDL fallback (issue #7). Absent in dev: api logs a warning and falls
  through to canonical-only mode (the search still works against the
  existing person graph; PDL filter dimensions are honoured against
  whatever the canonical already has)
- `UNSUBSCRIBE_SIGNING_SECRET` — required by both api and worker.
  HS256 key that signs the RFC 8058 List-Unsubscribe tokens on
  outbound campaign messages and verifies them at the public
  `/api/v1/public/unsubscribe` endpoint. Both processes MUST share
  the same value; any opaque 32+ byte string works
- `UNSUBSCRIBE_MAIL_DOMAIN` — required by the worker. Bare hostname
  used in the mailto channel of List-Unsubscribe (e.g.
  `mail.magiklead.com`). See the operational gap below
- `UNIPILE_API_KEY` (optional) — Unipile key for the LinkedIn send rail
  (docs/2026-06-09-linkedin-only-outreach). Absent in dev: the LinkedIn
  connect/send routes degrade like PDL — `GET /linkedin/auth-url`
  returns 503 `linkedin_disabled` instead of crashing. A dev dummy value
  is enough to exercise the connect gate + webhook locally
- `UNIPILE_DSN` (optional) — the per-tenant Unipile API base URL /
  subdomain (e.g. `https://api6.unipile.com:13443`). Used as the base
  for hosted-auth + disconnect calls
- `UNIPILE_WEBHOOK_SECRET` — required once `UNIPILE_API_KEY` is set (api
  fails fast otherwise). HS256 key that does double duty: it signs the
  `pkg/jwt` metadata token embedded in the hosted-auth link (which
  Unipile echoes back on `account.connected`, binding the connected
  account to the tenant) AND verifies the HMAC-SHA256 signature on the
  inbound `/api/v1/webhooks/unipile` body. Keep it distinct from the
  other signing secrets; any opaque 32+ byte string. The public webhook
  returns 503 if it is unset and 401 on a bad signature

`~/.config/devpods/magiklead/.env.frontend`:

- `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY`
- `CLERK_SECRET_KEY` (Clerk's Next.js middleware reads it server-side)

Devpods generates `DATABASE_URL`, `REDIS_URL`, `S3_*`, `APP_URL`,
`FRONTEND_URL`, and (when the stripe module is enabled)
`STRIPE_WEBHOOK_SECRET` automatically — do NOT set those in
`.env.backend`. The api compose entrypoint reads
`/shared/stripe-webhook-secret` (written by the `stripe-listen`
container) and exports `STRIPE_WEBHOOK_SECRET` before starting air.

### Known operational gaps

- **Unsubscribe mailto channel is advertised but not processed.**
  Every outbound campaign message carries
  `List-Unsubscribe: <mailto:unsubscribe+<token>@$UNSUBSCRIBE_MAIL_DOMAIN>, <https://…>`
  because Gmail's deliverability gate requires both channels to be
  present. There is no inbound-email infrastructure in this release,
  so emails sent to that mailto address pile up unanswered. The
  https one-click channel is the only path that actually records
  the unsubscribe today. Wire inbound parsing in a follow-up before
  scaling beyond the seven-day launch window.

### Clerk dashboard prereqs

1. Create a JWT template named **`magiklead-backend`** with body
   `{"email": "{{user.primary_email_address}}"}`. Without it the
   backend's admin gate returns 403 "Missing email claim".
2. Add a webhook endpoint with events `user.created` + `user.updated`
   pointing at `http://api-<devpod>.magiklead.localhost/api/v1/webhooks/clerk`.
   Clerk can't reach `.localhost` directly — for local testing, expose
   it via `cloudflared tunnel run` or `ngrok http 80` and use the
   tunnel URL.
