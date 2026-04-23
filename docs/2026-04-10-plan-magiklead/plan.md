# MagikLead — 7-Day Implementation Plan

**Date:** 2026-04-10
**Product:** MagikLead — AI-powered outbound sales automation (MoneyPrinter clone)
**Research:** [docs/research.md](../research.md)
**Tech Stack:** Go (Chi v5) + Next.js 16 + Tailwind + Shadcn + PostgreSQL + Redis (Asynq) + Clerk + Claude API + Gmail API
**Repo structure:** Monorepo (`backend/` + `frontend/`)

---

## Scope Decision

The research doc defines 4 phases. This plan covers **Phase 1 (core email pipeline) + landing page + billing** — enough to validate the product and start collecting revenue.

### In Scope (7-day MVP)

- Paste website URL → AI generates business profile
- AI generates 3-5 sales plays from profile
- Lead discovery (RapidAPI LinkedIn search → email finding → verification → cache)
- AI generates email sequences for leads
- Gmail OAuth + send emails via Gmail API
- Asynq-powered sequence execution engine
- Campaign management (create, start, pause, view stats)
- Basic dashboard (campaigns, leads, sent/replied/bounced counts)
- Stripe billing (Free/Starter $49/Growth $149/Scale $399)
- Landing page + pricing + legal pages
- SEO comparison pages

### Deferred to Phase 2

| Feature | Reason |
|---------|--------|
| Chrome Extension (LinkedIn automation) | Separate product surface, 2-3 days on its own |
| Email open/click tracking | Requires hosted pixel/redirect service — adds complexity, not core to validation |
| Conversations view | Needs reply detection working first |
| Campaign analytics charts | Basic counts are sufficient for MVP |
| Sequence editor UI | Users can regenerate; editing is nice-to-have |
| Lead CSV upload | API-driven discovery is the differentiator |
| Multi-tenant switching UI | First tenant (MagikShot) is hardcoded; switching comes later |
| Reply detection (Gmail polling) | Can be added week 2 without schema changes |

---

## Day-by-Day Overview

| Day | Focus | Tasks | Hours |
|-----|-------|-------|-------|
| 1 | Foundation | T01-T03 (scaffolding, DB, infra) | ~8h |
| 2 | Landing Page + Design System | T04-T05 (landing, pricing) | ~7h |
| 3 | App UI | T06-T08 (layout, onboarding, dashboard) | ~10h |
| 4 | Backend Core | T09-T12 (server, AI, leads, campaigns) | ~12h |
| 5 | Integration + Payments | T13-T16 (Gmail, wiring, Stripe, auth) | ~12h |
| 6 | SEO + Content | T17-T18 (comparison pages, legal, technical SEO) | ~6h |
| 7 | Polish + Deploy | T19-T20 (deploy, QA, launch prep) | ~6h |

---

## Tasks

---

### T01: Project Scaffolding

**Day:** 1
**Estimated time:** 2 hours
**Depends on:** None
**Skill/Agent:** Manual / Claude Code

#### Context

Set up the monorepo structure with Go backend and Next.js 16 frontend. This task creates every directory, config file, and development tool configuration. Nothing builds on nothing — this is the foundation that every subsequent task depends on.

The research doc specifies Go + Chi v5 for backend and Next.js 16 + Tailwind + Shadcn for frontend. We match the MagikShot stack so patterns can be reused.

#### Inputs

- Research doc Section 3 (Architecture → Repository Structure)
- Go 1.22+, Node 20+, pnpm installed locally

#### Deliverables

- `backend/go.mod`, `backend/go.sum`
- `backend/cmd/api/main.go` (entrypoint stub)
- `backend/cmd/worker/main.go` (Asynq worker stub)
- `backend/internal/` directory tree (handler, service, repository, leads, ai, gmail, campaign, worker)
- `backend/pkg/dto/`, `backend/pkg/errors/`
- `backend/migrations/` (empty, ready for T02)
- `backend/queries/` (empty, ready for SQLC)
- `backend/sqlc.yaml`
- `backend/Makefile`
- `frontend/` — Next.js 16 app with Tailwind + Shadcn configured
- `frontend/src/app/` route stubs
- `frontend/src/lib/api.ts` (API client stub)
- `.env.example` (all environment variables documented)
- `docker-compose.yml` (dev: Postgres + Redis)
- `Makefile` (root: `make dev`, `make build`, `make migrate`)

#### Specification

**Backend (Go):**

```bash
cd backend
go mod init github.com/jcleira/magiklead/backend
```

Dependencies to install:
```
github.com/go-chi/chi/v5
github.com/go-chi/cors
github.com/hibiken/asynq
github.com/jackc/pgx/v5
github.com/clerk/clerk-sdk-go/v2
github.com/anthropics/anthropic-sdk-go
github.com/stripe/stripe-go/v81
github.com/joho/godotenv
golang.org/x/oauth2
google.golang.org/api
```

`cmd/api/main.go` — loads env, creates Postgres pool, creates Redis client, sets up Chi router, registers handlers, starts HTTP server on `:8080`.

`cmd/worker/main.go` — loads env, creates Postgres pool, creates Redis client, creates Asynq server, registers task handlers, starts worker.

SQLC config (`sqlc.yaml`):
```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "queries/"
    schema: "migrations/"
    gen:
      go:
        package: "repository"
        out: "internal/repository"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_empty_slices: true
```

**Frontend (Next.js 16):**

```bash
cd frontend
pnpm create next-app@latest . --typescript --tailwind --eslint --app --src-dir --import-alias "@/*"
pnpm dlx shadcn@latest init
```

Configure Shadcn with "new-york" style, slate base color. Install initial components:
```bash
pnpm dlx shadcn@latest add button card input label textarea select badge table tabs dialog sheet dropdown-menu separator skeleton toast
```

Install additional dependencies:
```bash
pnpm add @clerk/nextjs
pnpm add zustand
```

Create route stubs (empty page.tsx with placeholder text):
- `src/app/(marketing)/page.tsx` — landing page
- `src/app/(marketing)/pricing/page.tsx`
- `src/app/(app)/dashboard/page.tsx`
- `src/app/(app)/campaigns/page.tsx`
- `src/app/(app)/campaigns/[id]/page.tsx`
- `src/app/(app)/leads/page.tsx`
- `src/app/(app)/onboarding/page.tsx`
- `src/app/(app)/settings/page.tsx`
- `src/app/(app)/layout.tsx` — authenticated layout with sidebar
- `src/app/(marketing)/layout.tsx` — public layout with nav

Create `src/lib/api.ts`:
```typescript
const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
  });
  if (!res.ok) {
    throw new Error(`API error: ${res.status}`);
  }
  return res.json();
}
```

**Root configs:**

`.env.example`:
```
# Database
DATABASE_URL=postgres://magiklead:magiklead@localhost:5432/magiklead?sslmode=disable

# Redis
REDIS_URL=localhost:6379

# Clerk
CLERK_SECRET_KEY=sk_test_...
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=pk_test_...
CLERK_WEBHOOK_SECRET=whsec_...

# Claude API
ANTHROPIC_API_KEY=sk-ant-...

# RapidAPI
RAPIDAPI_KEY=...

# Gmail OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URI=http://localhost:3000/api/gmail/callback

# Stripe
STRIPE_SECRET_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY=pk_test_...

# App
NEXT_PUBLIC_API_URL=http://localhost:8080
FRONTEND_URL=http://localhost:3000
```

`docker-compose.yml`:
```yaml
services:
  postgres:
    image: postgres:16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_USER: magiklead
      POSTGRES_PASSWORD: magiklead
      POSTGRES_DB: magiklead
    volumes:
      - pgdata:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

volumes:
  pgdata:
```

Root `Makefile`:
```makefile
.PHONY: dev dev-be dev-fe dev-infra build migrate

dev-infra:
	docker compose up -d

dev-be:
	cd backend && go run ./cmd/api/...

dev-worker:
	cd backend && go run ./cmd/worker/...

dev-fe:
	cd frontend && pnpm dev

dev: dev-infra
	@echo "Run 'make dev-be', 'make dev-worker', and 'make dev-fe' in separate terminals"

migrate:
	cd backend && go run ./cmd/migrate/...

build:
	cd backend && go build -o bin/api ./cmd/api/...
	cd backend && go build -o bin/worker ./cmd/worker/...
	cd frontend && pnpm build
```

#### Acceptance Criteria

- [ ] `docker compose up -d` starts Postgres and Redis
- [ ] `cd backend && go build ./...` compiles without errors
- [ ] `cd frontend && pnpm build` builds without errors
- [ ] All directories from research doc's repo structure exist
- [ ] `.env.example` documents every required environment variable
- [ ] SQLC config is valid (`cd backend && sqlc compile` succeeds after T02 adds queries)

#### Notes

- Use Go workspace if needed for mono-repo, but a flat `backend/` + `frontend/` is simpler
- Don't add linting rules beyond Go defaults and ESLint defaults — keep it simple
- The `(marketing)` and `(app)` route groups in Next.js use different layouts but share the same domain

---

### T02: Database Schema + Migrations

**Day:** 1
**Estimated time:** 3 hours
**Depends on:** T01
**Skill/Agent:** Manual / Claude Code

#### Context

Create all database tables from the research doc's data model (Section 11). This includes 10 tables covering multi-tenancy, leads, campaigns, email tracking, LinkedIn tasks, Gmail accounts, and subscriptions. The schema is designed for zero-marginal-cost lead caching — the `leads` and `lead_searches` tables are tenant-agnostic so cached leads benefit all users.

We use golang-migrate for migrations and SQLC for type-safe query generation. Each table gets its own migration file for clean rollbacks.

#### Inputs

- Research doc Section 11 (Data Model — full SQL schema)
- Research doc Section 8 (Multi-Tenant Model)
- T01 scaffolding complete (backend/migrations/ directory exists)
- `DATABASE_URL` from `.env`

#### Deliverables

- `backend/migrations/001_create_tenants.up.sql` / `.down.sql`
- `backend/migrations/002_create_leads.up.sql` / `.down.sql`
- `backend/migrations/003_create_lead_searches.up.sql` / `.down.sql`
- `backend/migrations/004_create_gmail_accounts.up.sql` / `.down.sql`
- `backend/migrations/005_create_plays.up.sql` / `.down.sql`
- `backend/migrations/006_create_campaigns.up.sql` / `.down.sql`
- `backend/migrations/007_create_campaign_leads.up.sql` / `.down.sql`
- `backend/migrations/008_create_email_events.up.sql` / `.down.sql`
- `backend/migrations/009_create_linkedin_tasks.up.sql` / `.down.sql`
- `backend/migrations/010_create_subscriptions.up.sql` / `.down.sql`
- `backend/migrations/011_create_users.up.sql` / `.down.sql`
- `backend/queries/*.sql` (SQLC query files for each table)
- `backend/internal/repository/` (SQLC-generated Go code)
- `backend/cmd/migrate/main.go` (migration runner)

#### Specification

Use the exact SQL from research doc Section 11 with these additions:

**Migration 011 — users table** (not in research but needed for Clerk → DB mapping):
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_id TEXT UNIQUE NOT NULL,
    email TEXT NOT NULL,
    name TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Link users to tenants (many-to-many for multi-tenant)
CREATE TABLE user_tenants (
    user_id UUID NOT NULL REFERENCES users(id),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    role TEXT DEFAULT 'owner',  -- owner, admin, member
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, tenant_id)
);
```

**Add indexes to all tables:**
```sql
-- leads
CREATE INDEX idx_leads_email ON leads(email);
CREATE INDEX idx_leads_company_domain ON leads(company_domain);
CREATE INDEX idx_leads_linkedin_url ON leads(linkedin_url);

-- plays
CREATE INDEX idx_plays_tenant_id ON plays(tenant_id);

-- campaigns
CREATE INDEX idx_campaigns_tenant_id ON campaigns(tenant_id);
CREATE INDEX idx_campaigns_status ON campaigns(status);

-- campaign_leads
CREATE INDEX idx_campaign_leads_campaign_id ON campaign_leads(campaign_id);
CREATE INDEX idx_campaign_leads_lead_id ON campaign_leads(lead_id);
CREATE INDEX idx_campaign_leads_next_send_at ON campaign_leads(next_send_at) WHERE status = 'active';

-- email_events
CREATE INDEX idx_email_events_campaign_lead_id ON email_events(campaign_lead_id);

-- linkedin_tasks
CREATE INDEX idx_linkedin_tasks_status ON linkedin_tasks(status) WHERE status = 'pending';

-- subscriptions
CREATE INDEX idx_subscriptions_tenant_id ON subscriptions(tenant_id);
```

**SQLC queries** — create one `.sql` file per table in `backend/queries/`:

`queries/tenants.sql`:
```sql
-- name: CreateTenant :one
INSERT INTO tenants (name, domain, business_profile)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: UpdateTenantProfile :one
UPDATE tenants SET business_profile = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ListTenantsByUser :many
SELECT t.* FROM tenants t
JOIN user_tenants ut ON t.id = ut.tenant_id
WHERE ut.user_id = $1;
```

`queries/leads.sql`:
```sql
-- name: CreateLead :one
INSERT INTO leads (linkedin_url, first_name, last_name, title, company, company_domain, location, email, email_verified, source, raw_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (linkedin_url) DO UPDATE SET
    title = EXCLUDED.title,
    company = EXCLUDED.company,
    email = COALESCE(EXCLUDED.email, leads.email),
    enriched_at = NOW()
RETURNING *;

-- name: GetLead :one
SELECT * FROM leads WHERE id = $1;

-- name: ListLeadsByIDs :many
SELECT * FROM leads WHERE id = ANY($1::uuid[]);

-- name: SearchLeadsByCompanyDomain :many
SELECT * FROM leads WHERE company_domain = $1;
```

`queries/lead_searches.sql`:
```sql
-- name: GetCachedSearch :one
SELECT * FROM lead_searches
WHERE query_hash = $1
AND fetched_at > NOW() - (ttl_days || ' days')::interval
ORDER BY fetched_at DESC
LIMIT 1;

-- name: CreateLeadSearch :one
INSERT INTO lead_searches (query_hash, filters, result_lead_ids, fetched_at)
VALUES ($1, $2, $3, NOW())
RETURNING *;
```

`queries/campaigns.sql`:
```sql
-- name: CreateCampaign :one
INSERT INTO campaigns (tenant_id, play_id, name, status, gmail_account_id, sequence, linkedin_sequence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = $1 AND tenant_id = $2;

-- name: ListCampaigns :many
SELECT * FROM campaigns WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdateCampaignStatus :one
UPDATE campaigns SET status = $2 WHERE id = $1 RETURNING *;

-- name: UpdateCampaignStats :exec
UPDATE campaigns SET stats = $2 WHERE id = $1;
```

`queries/campaign_leads.sql`:
```sql
-- name: AddLeadToCampaign :one
INSERT INTO campaign_leads (campaign_id, lead_id, status, next_send_at)
VALUES ($1, $2, 'queued', $3)
ON CONFLICT (campaign_id, lead_id) DO NOTHING
RETURNING *;

-- name: GetDueLeads :many
SELECT cl.*, l.email, l.first_name, l.last_name, l.company, l.title
FROM campaign_leads cl
JOIN leads l ON cl.lead_id = l.id
WHERE cl.next_send_at <= NOW()
AND cl.status = 'active'
ORDER BY cl.next_send_at
LIMIT $1;

-- name: UpdateCampaignLeadStep :exec
UPDATE campaign_leads
SET current_step = $2, next_send_at = $3, last_sent_at = NOW(), status = $4
WHERE id = $1;

-- name: ListCampaignLeads :many
SELECT cl.*, l.first_name, l.last_name, l.email, l.title, l.company, l.linkedin_url
FROM campaign_leads cl
JOIN leads l ON cl.lead_id = l.id
WHERE cl.campaign_id = $1
ORDER BY cl.created_at DESC;

-- name: CountCampaignLeadsByStatus :many
SELECT status, COUNT(*) as count FROM campaign_leads
WHERE campaign_id = $1
GROUP BY status;
```

`queries/plays.sql`:
```sql
-- name: CreatePlay :one
INSERT INTO plays (tenant_id, name, description, icp, search_query, channels, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetPlay :one
SELECT * FROM plays WHERE id = $1 AND tenant_id = $2;

-- name: ListPlays :many
SELECT * FROM plays WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdatePlay :one
UPDATE plays SET name = $2, description = $3, icp = $4, search_query = $5, channels = $6, status = $7
WHERE id = $1 RETURNING *;

-- name: DeletePlay :exec
DELETE FROM plays WHERE id = $1 AND tenant_id = $2;
```

`queries/gmail_accounts.sql`:
```sql
-- name: CreateGmailAccount :one
INSERT INTO gmail_accounts (tenant_id, user_id, email, access_token, refresh_token, token_expiry)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetGmailAccount :one
SELECT * FROM gmail_accounts WHERE id = $1 AND tenant_id = $2;

-- name: ListGmailAccounts :many
SELECT * FROM gmail_accounts WHERE tenant_id = $1;

-- name: UpdateGmailTokens :exec
UPDATE gmail_accounts SET access_token = $2, refresh_token = $3, token_expiry = $4 WHERE id = $1;

-- name: IncrementDailySent :exec
UPDATE gmail_accounts SET daily_sent_count = daily_sent_count + 1 WHERE id = $1;

-- name: ResetDailySentCounts :exec
UPDATE gmail_accounts SET daily_sent_count = 0, daily_sent_reset_at = NOW()
WHERE daily_sent_reset_at < CURRENT_DATE;

-- name: DeleteGmailAccount :exec
DELETE FROM gmail_accounts WHERE id = $1 AND tenant_id = $2;
```

`queries/email_events.sql`:
```sql
-- name: CreateEmailEvent :one
INSERT INTO email_events (campaign_lead_id, event_type, step, metadata)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListEmailEvents :many
SELECT * FROM email_events WHERE campaign_lead_id = $1 ORDER BY created_at;
```

`queries/subscriptions.sql`:
```sql
-- name: CreateSubscription :one
INSERT INTO subscriptions (tenant_id, plan, leads_limit, sequences_limit, stripe_subscription_id, current_period_start, current_period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE tenant_id = $1;

-- name: UpdateSubscriptionPlan :exec
UPDATE subscriptions SET plan = $2, leads_limit = $3, sequences_limit = $4, stripe_subscription_id = $5, current_period_start = $6, current_period_end = $7 WHERE tenant_id = $1;

-- name: IncrementLeadsUsed :exec
UPDATE subscriptions SET leads_used = leads_used + $2 WHERE tenant_id = $1;

-- name: IncrementSequencesUsed :exec
UPDATE subscriptions SET sequences_used = sequences_used + $2 WHERE tenant_id = $1;

-- name: ResetUsageCounts :exec
UPDATE subscriptions SET leads_used = 0, sequences_used = 0
WHERE current_period_end < NOW();
```

`queries/users.sql`:
```sql
-- name: CreateUser :one
INSERT INTO users (clerk_id, email, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetUserByClerkID :one
SELECT * FROM users WHERE clerk_id = $1;

-- name: CreateUserTenant :exec
INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, $3);
```

**Migration runner** (`backend/cmd/migrate/main.go`):
```go
package main

import (
    "database/sql"
    "log"
    "os"

    "github.com/golang-migrate/migrate/v4"
    "github.com/golang-migrate/migrate/v4/database/postgres"
    _ "github.com/golang-migrate/migrate/v4/source/file"
    _ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
    db, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal(err)
    }
    driver, err := postgres.WithInstance(db, &postgres.Config{})
    if err != nil {
        log.Fatal(err)
    }
    m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
    if err != nil {
        log.Fatal(err)
    }
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        log.Fatal(err)
    }
    log.Println("Migrations applied successfully")
}
```

After creating all files, run:
```bash
cd backend && sqlc generate
```

#### Acceptance Criteria

- [ ] `make migrate` applies all 11 migrations to local Postgres without errors
- [ ] `sqlc generate` produces Go code in `backend/internal/repository/` without errors
- [ ] All tables exist in the database with correct columns, types, and indexes
- [ ] Down migrations cleanly drop all tables
- [ ] Generated Go code compiles without errors

#### Notes

- Use `TIMESTAMPTZ` everywhere, never `TIMESTAMP` — always store UTC
- The `business_profile` JSONB column on tenants stores the AI-analyzed website data — no separate table needed
- `campaign_leads.next_send_at` index is partial (WHERE status = 'active') for worker performance
- `leads.linkedin_url` has a UNIQUE constraint — upsert on conflict for deduplication

---

### T03: Infrastructure Setup

**Day:** 1
**Estimated time:** 2 hours
**Depends on:** T01
**Skill/Agent:** Manual / Claude Code

#### Context

Create the Docker, deployment, and local development configurations. This includes the production Dockerfile for the Go backend (multi-stage build), the Vercel configuration for the frontend, and environment variable management.

#### Inputs

- T01 scaffolding complete
- Hetzner server specs from research (smallest VPS for launch: ~$5-20/mo)

#### Deliverables

- `backend/Dockerfile` (multi-stage Go build)
- `backend/Dockerfile.worker` (Asynq worker)
- `docker-compose.prod.yml` (production: API + worker + Postgres + Redis + Caddy)
- `frontend/vercel.json` (rewrites, env)
- `backend/.air.toml` (hot reload for Go development)
- `.github/workflows/deploy.yml` (CI/CD placeholder)

#### Specification

**`backend/Dockerfile`** (multi-stage):
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api/...

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /api /api
COPY migrations /migrations
EXPOSE 8080
CMD ["/api"]
```

**`backend/Dockerfile.worker`:**
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./cmd/worker/...

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /worker /worker
CMD ["/worker"]
```

**`docker-compose.prod.yml`:**
```yaml
services:
  api:
    build:
      context: ./backend
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    env_file: .env
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  worker:
    build:
      context: ./backend
      dockerfile: Dockerfile.worker
    env_file: .env
    depends_on:
      - postgres
      - redis
    restart: unless-stopped

  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: magiklead
    volumes:
      - pgdata:/var/lib/postgresql/data
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    restart: unless-stopped

  caddy:
    image: caddy:2-alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
    depends_on:
      - api
    restart: unless-stopped

volumes:
  pgdata:
  caddy_data:
```

**`Caddyfile`:**
```
api.magiklead.com {
    reverse_proxy api:8080
}
```

**`backend/.air.toml`** (hot reload):
```toml
root = "."
[build]
  cmd = "go build -o ./tmp/api ./cmd/api/..."
  bin = "tmp/api"
  include_ext = ["go"]
  exclude_dir = ["tmp", "vendor"]
[log]
  time = true
```

**`frontend/vercel.json`:**
```json
{
  "rewrites": [
    { "source": "/api/:path*", "destination": "https://api.magiklead.com/api/:path*" }
  ]
}
```

#### Acceptance Criteria

- [ ] `docker build -f backend/Dockerfile backend/` succeeds
- [ ] `docker build -f backend/Dockerfile.worker backend/` succeeds
- [ ] `docker compose -f docker-compose.prod.yml config` validates without errors
- [ ] Air hot-reload works: `cd backend && air` starts and rebuilds on file changes
- [ ] `.env.example` matches all variables referenced in docker-compose files

#### Notes

- Caddy handles automatic HTTPS via Let's Encrypt — no manual SSL setup needed
- For Neon (managed Postgres), remove the postgres service from prod compose and use `DATABASE_URL` pointing to Neon
- The production compose assumes self-hosted Postgres; switch to Neon connection string for managed option

---

### T04: Landing Page

**Day:** 2
**Estimated time:** 4 hours
**Depends on:** T01
**Skill/Agent:** Claude Code

#### Context

The landing page is the first real deliverable and the most important page in the product. It establishes the visual identity, design tokens, and messaging that every subsequent page inherits. The research doc's brand strategy (Section 17) defines the tone as "direct, no-BS, builder-friendly" with the headline "AI outreach that doesn't cost a fortune."

This page needs to convert visitors from Product Hunt, Hacker News, and Google Ads into signups. The target persona is solo founders and small sales teams (1-5 people) at early-stage startups doing their own outbound.

#### Inputs

- Research doc Section 17 (Domain & Brand Strategy — persona, tone, headlines, value props)
- Research doc Section 16 (Differentiation Strategy — positioning, why switch)
- Research doc Section 1 (What MoneyPrinter Does — to match the flow)
- Research doc Section 10 (Pricing Model — tiers for pricing section)
- T01 frontend scaffolding

#### Deliverables

- `frontend/src/app/(marketing)/page.tsx` — landing page
- `frontend/src/app/(marketing)/layout.tsx` — marketing layout (nav + footer)
- `frontend/src/components/marketing/` — reusable marketing components
- `frontend/src/app/globals.css` — design tokens (CSS variables)
- `docs/design-tokens.md` — reference document for all subsequent UI tasks

#### Specification

**Design Brief:**
- **Purpose:** Convert Product Hunt / HN / Google Ads traffic into signups
- **Target audience:** Solo founders, small sales teams (1-5), early-stage startups doing outbound
- **Aesthetic:** Clean, modern SaaS. Think Linear meets Vercel — dark option available but default light. No gradients, no 3D illustrations. Sharp, typographic, confident.
- **What makes it memorable:** Transparent pricing comparison showing we're 50% cheaper. Live "how it works" demo showing the paste-URL-to-leads flow.
- **Tone:** Direct, confident, no-BS. "Here's what we do, here's what it costs, here's why it's better."

**Page Structure:**

1. **Nav bar** — Logo (MagikLead), Links: Features, Pricing, Docs. Right side: Sign In (Clerk), CTA "Start Free"

2. **Hero section:**
   - Headline: "AI outreach that doesn't cost a fortune"
   - Subheadline: "Paste your website. Get leads. Send personalized emails and LinkedIn messages. Starting at $49/mo."
   - Primary CTA: "Start Free" → Clerk sign-up
   - Secondary CTA: "See How It Works" → scrolls to demo section
   - Social proof bar: "Backed by [X] founders" or trust badges (YC-style if applicable)

3. **"How it works" section** (3-step visual):
   - Step 1: "Paste your website" — AI analyzes your product, pricing, customers
   - Step 2: "Discover leads" — Find decision-makers matching your ICP
   - Step 3: "Send sequences" — AI-written emails sent from your Gmail

4. **Value propositions** (3 cards):
   - "All-in-one pipeline" — Leads + email + LinkedIn in a single tool. No duct-taping 3 platforms together.
   - "50% cheaper" — Same AI-powered outreach at a fraction of the cost.
   - "Open and transparent" — No credit gotchas, no per-seat surprises.

5. **Competitor comparison table:**
   - Columns: Feature, MagikLead, MoneyPrinter, Apollo, Instantly, Lemlist
   - Rows: Price, Channels, Lead Discovery, AI Sequences, LinkedIn, Email Accounts
   - MagikLead column highlighted

6. **Pricing section** (embed or link to T05):
   - 4 tier cards: Free, Starter ($49), Growth ($149), Scale ($399)
   - Feature comparison per tier
   - Annual toggle (20% discount)

7. **FAQ section:**
   - "How does lead discovery work?"
   - "Do I need a separate email tool?"
   - "Is my Gmail safe?"
   - "How does billing work?"
   - "Can I use this with LinkedIn?"

8. **Final CTA section:**
   - "Ready to print leads?" (callback to MoneyPrinter naming)
   - "Start free — no credit card required"

9. **Footer:**
   - Product links, Legal links (Privacy, Terms), Social links, Copyright

**Design Tokens to Establish (write to `docs/design-tokens.md`):**
- Font: Inter or Geist Sans for body, Geist Mono for code/numbers
- Color palette: slate base, primary blue (Tailwind blue-600), success green, warning amber, error red
- Spacing scale: 4px base (Tailwind default)
- Border radius: rounded-lg for cards, rounded-md for buttons, rounded-full for badges
- Shadows: subtle, single-layer (shadow-sm for cards, shadow-md for dropdowns)
- Component patterns: card with border (not shadow), button sizes (sm, md, lg), form field styles

#### Acceptance Criteria

- [ ] Landing page renders at `/` with all 9 sections
- [ ] "Start Free" CTA links to Clerk sign-up
- [ ] Competitor comparison table shows accurate pricing from research doc
- [ ] Mobile responsive — all sections readable on 375px width
- [ ] `docs/design-tokens.md` exists and documents fonts, colors, spacing, component patterns
- [ ] Page loads in < 2s on simulated 3G (Lighthouse)
- [ ] No Tailwind classes that conflict with the established design tokens

#### Notes

- Use Next.js Server Components for the marketing pages — no client-side JS needed except for the mobile menu toggle and annual pricing toggle
- The pricing section can be a separate component imported from T05 or built inline — decide based on complexity
- Don't over-animate. One subtle entrance animation on the hero is fine. No scroll-triggered animations.

---

### T05: Pricing Page/Section

**Day:** 2
**Estimated time:** 3 hours
**Depends on:** T04
**Skill/Agent:** Claude Code

#### Context

The pricing page is critical for conversion. It must clearly communicate the value of each tier and make the Starter ($49/mo) tier the obvious choice for most users. The research doc shows we're 50% cheaper than MoneyPrinter across all tiers — this needs to be visible.

#### Inputs

- Research doc Section 10 (Pricing Model — exact tiers, limits, features)
- Research doc Section 16 (Differentiation — price comparison with competitors)
- T04 design tokens (`docs/design-tokens.md`)
- T04 marketing layout

#### Deliverables

- `frontend/src/app/(marketing)/pricing/page.tsx`
- `frontend/src/components/marketing/pricing-card.tsx`
- `frontend/src/components/marketing/pricing-comparison.tsx`

#### Specification

**Design Brief:**
- **Purpose:** Convert free-tier browsers into paid subscribers
- **Aesthetic:** Inherits from T04 design tokens. Clean tier cards, highlighted "Most Popular" on Growth tier.
- **Tone:** Transparent, no-gotchas. Show exactly what you get.

**Pricing Tiers (from research):**

| Plan | $/mo | Annual | Leads/mo | Sequences/mo | Campaigns | Features |
|------|------|--------|----------|-------------|-----------|----------|
| Free | $0 | $0 | 100 | 300 | 1 | Email only, 1 Gmail |
| Starter | $49 | $39 | 500 | 1,500 | 3 | Email + LinkedIn, 1 Gmail |
| Growth | $149 | $119 | 2,000 | 6,000 | 10 | Email + LinkedIn, 3 Gmail, AI personalization |
| Scale | $399 | $319 | 10,000 | 30,000 | Unlimited | Everything + priority support, API access |

**Page Layout:**

1. **Header:** "Simple, transparent pricing"
   - Subheadline: "Start free. Upgrade when you need more leads."
   - Annual/Monthly toggle

2. **4 Tier Cards side by side:**
   - Plan name, price, billing period
   - Primary feature highlight (e.g., "500 leads/mo" for Starter)
   - Feature list with checkmarks
   - CTA button: "Start Free" (Free), "Get Started" (Starter), "Get Started" (Growth, highlighted), "Contact Us" (Scale)
   - Growth card has "Most Popular" badge

3. **"Save with competitors" comparison:**
   - Side-by-side: "What you'd pay elsewhere"
   - MoneyPrinter Growth: $250/mo → MagikLead Growth: $149/mo → Save $101/mo
   - Lemlist Multichannel: $109/user/mo → MagikLead Starter: $49/mo → Save $60/mo

4. **Feature comparison table** (expandable):
   - All features listed with checkmarks per tier
   - Categories: Lead Discovery, Email, LinkedIn, AI, Analytics, Support

5. **FAQ specific to pricing:**
   - "What counts as a lead?"
   - "Do unused leads roll over?"
   - "Can I change plans anytime?"
   - "What payment methods do you accept?"

#### Acceptance Criteria

- [ ] Pricing page renders at `/pricing` with all 4 tier cards
- [ ] Annual/monthly toggle updates prices dynamically (client component)
- [ ] "Most Popular" badge on Growth tier
- [ ] Competitor savings comparison is visible
- [ ] Feature comparison table is accurate per research doc
- [ ] Mobile: tier cards stack vertically
- [ ] CTA buttons link to Clerk sign-up (free) or Stripe checkout (paid — wired in T15)

#### Notes

- Annual prices are 20% off monthly: Starter $39, Growth $119, Scale $319
- The "Contact Us" for Scale tier can be a mailto: for MVP
- Prices should be stored as constants in a shared config, not hardcoded in JSX

---

### T06: App Layout + Navigation

**Day:** 3
**Estimated time:** 2 hours
**Depends on:** T04
**Skill/Agent:** Claude Code

#### Context

Build the authenticated application shell — the sidebar navigation, top bar, and responsive layout that wraps all app pages (dashboard, campaigns, leads, settings). This layout is what users see after signing in. It must match the design language established by the landing page (T04).

The research doc's MoneyPrinter dashboard analysis (Section 1) shows the navigation structure we're replicating.

#### Inputs

- Research doc Section 1 (MoneyPrinter Dashboard Features — sidebar nav items)
- T04 design tokens (`docs/design-tokens.md`)
- Clerk Next.js integration (ClerkProvider, UserButton, auth middleware)

#### Deliverables

- `frontend/src/app/(app)/layout.tsx` — authenticated layout
- `frontend/src/components/app/sidebar.tsx` — sidebar navigation
- `frontend/src/components/app/top-bar.tsx` — top bar with user menu
- `frontend/src/middleware.ts` — Clerk auth middleware (protect /dashboard/*, /campaigns/*, etc.)

#### Specification

**Design Brief:**
- **Purpose:** Wrap all authenticated app pages with consistent navigation
- **Aesthetic:** Sidebar dark or light (match design tokens from T04). Clean, minimal. Think Linear or Vercel dashboard.
- **Tone:** Professional, focused. No distractions.

**Sidebar Nav Items:**
```
Dashboard          (icon: LayoutDashboard)
Campaigns          (icon: Megaphone)
Leads              (icon: Users)
Settings           (icon: Settings)
---
Plan: Starter      (current plan badge)
```

**Top Bar:**
- Left: Page title (dynamic, based on route)
- Right: Clerk `<UserButton />` component

**Responsive:**
- Desktop (≥1024px): Fixed sidebar (240px) + content area
- Tablet/Mobile (<1024px): Hamburger menu → slide-out sidebar (Sheet component)

**Clerk Middleware** (`frontend/src/middleware.ts`):
```typescript
import { clerkMiddleware, createRouteMatcher } from "@clerk/nextjs/server";

const isProtectedRoute = createRouteMatcher([
  "/dashboard(.*)",
  "/campaigns(.*)",
  "/leads(.*)",
  "/settings(.*)",
  "/onboarding(.*)",
]);

export default clerkMiddleware(async (auth, req) => {
  if (isProtectedRoute(req)) {
    await auth.protect();
  }
});

export const config = {
  matcher: ["/((?!.*\\..*|_next).*)", "/", "/(api|trpc)(.*)"],
};
```

#### Acceptance Criteria

- [ ] Unauthenticated users visiting `/dashboard` are redirected to Clerk sign-in
- [ ] Authenticated users see sidebar with all nav items
- [ ] Active nav item is highlighted
- [ ] Clerk UserButton shows user avatar/name with sign-out option
- [ ] Mobile: sidebar collapses to hamburger menu
- [ ] Layout matches design tokens from T04

#### Notes

- Don't build the page contents yet — just the shell/layout. T07 and T08 fill the pages.
- Sidebar width should be a CSS variable for easy adjustment
- The plan badge in sidebar reads from subscription context (mocked for now, wired in T15)

---

### T07: Onboarding Flow UI

**Day:** 3
**Estimated time:** 4 hours
**Depends on:** T06
**Skill/Agent:** Claude Code

#### Context

The onboarding flow is the user's first experience after signing up. It replicates MoneyPrinter's core UX: paste a website URL, the AI analyzes it and generates a business profile, then generates sales plays. This is the "aha moment" — the user goes from nothing to a list of potential leads in minutes.

For MVP, this is a multi-step wizard. Backend calls will be mocked with loading states until T14 (integration).

#### Inputs

- Research doc Section 1 (MoneyPrinter onboarding flow)
- Research doc Section 7 (AI Layer — website analysis output shape, play generation output shape)
- T06 app layout
- T04 design tokens

#### Deliverables

- `frontend/src/app/(app)/onboarding/page.tsx` — multi-step wizard
- `frontend/src/components/app/onboarding/step-url.tsx` — Step 1: paste URL
- `frontend/src/components/app/onboarding/step-profile.tsx` — Step 2: review/edit business profile
- `frontend/src/components/app/onboarding/step-plays.tsx` — Step 3: review generated plays
- `frontend/src/components/app/onboarding/step-confirm.tsx` — Step 4: confirm and start discovering leads
- `frontend/src/stores/onboarding.ts` — Zustand store for wizard state

#### Specification

**Design Brief:**
- **Purpose:** First-time user activation. From zero to "my first sales play" in under 5 minutes.
- **Audience:** Non-technical founders. The UI must feel simple, not overwhelming.
- **Aesthetic:** Clean wizard with progress indicator. Each step is focused on one action.
- **What makes it memorable:** The "magic" moment when AI generates a business profile and plays from just a URL.
- **Tone:** Encouraging, forward-moving. "Let's go", "Looking good", "Almost there."

**Step 1: Paste Website URL**
- Large centered input: "Enter your website URL"
- Placeholder: "https://yourcompany.com"
- CTA: "Analyze" button
- Loading state: "Analyzing your website..." with animated dots and a progress description ("Scraping content...", "Identifying product...", "Generating profile...")
- Note: "Don't have a website? You can describe your product manually." → link to manual entry mode

**Step 2: Review Business Profile**
- Display AI-generated profile:
  - Company name (editable)
  - Product description (editable textarea)
  - Features list (editable, add/remove)
  - Pricing (editable)
  - Target customers (editable tags)
  - Industry (editable)
- CTA: "Looks good — Generate plays"
- Secondary: "Edit" toggle to modify fields

**Step 3: Review Sales Plays**
- Display 3-5 AI-generated plays as cards:
  - Play name (e.g., "HR Leaders at Hiring Tech Companies")
  - Target titles (chips/badges)
  - Company criteria (size, industry, geo)
  - Signal (what triggers outreach)
  - Channels (email, LinkedIn badges)
- Each play has a checkbox to select/deselect
- CTA: "Start with selected plays"

**Step 4: Confirm & Start**
- Summary of selected plays
- "We'll discover leads matching these plays and generate personalized email sequences."
- CTA: "Start Discovering Leads" → redirects to `/campaigns`
- Creates tenant, plays, and first campaign in the backend

**Zustand Store (`stores/onboarding.ts`):**
```typescript
interface OnboardingState {
  step: 1 | 2 | 3 | 4;
  websiteUrl: string;
  profile: BusinessProfile | null;
  plays: Play[];
  selectedPlayIds: string[];
  isLoading: boolean;
  error: string | null;
  setStep: (step: number) => void;
  setWebsiteUrl: (url: string) => void;
  setProfile: (profile: BusinessProfile) => void;
  setPlays: (plays: Play[]) => void;
  togglePlay: (id: string) => void;
  analyzeWebsite: () => Promise<void>;
  generatePlays: () => Promise<void>;
  confirm: () => Promise<void>;
}
```

**Mock data for pre-integration:**
- Use realistic mock business profile (from research doc — MagikShot example)
- Use realistic mock plays (from research doc Section 7)
- Simulate 2-3 second loading delays to match real API timing

#### Acceptance Criteria

- [ ] 4-step wizard with progress indicator
- [ ] Step 1: URL input with validation (must start with http/https)
- [ ] Step 2: Editable business profile fields
- [ ] Step 3: Play cards with select/deselect
- [ ] Step 4: Summary and confirmation
- [ ] Loading states between steps (simulated)
- [ ] Can navigate back to previous steps
- [ ] Mobile responsive — wizard works on 375px width
- [ ] Mock data looks realistic

#### Notes

- The Zustand store should be persisted to localStorage so refreshing doesn't lose progress
- The "manual entry" mode on Step 1 is a stretch goal — just the URL input is fine for MVP
- Step 2 and 3 are the "aha moment" — make the AI output look impressive

---

### T08: Campaigns & Leads Dashboard UI

**Day:** 3
**Estimated time:** 4 hours
**Depends on:** T06
**Skill/Agent:** Claude Code

#### Context

The campaigns and leads pages are the daily-use screens. After onboarding, users come here to manage their outreach: view campaign status, see lead lists, check sent/replied/bounced counts. The research doc's MoneyPrinter analysis (Section 1) shows the campaign view layout we're replicating.

#### Inputs

- Research doc Section 1 (Campaign view — metrics, contact list, sequence preview)
- Research doc Section 12 (API Design — campaign and lead endpoints)
- T06 app layout
- T04 design tokens

#### Deliverables

- `frontend/src/app/(app)/dashboard/page.tsx` — overview dashboard
- `frontend/src/app/(app)/campaigns/page.tsx` — campaigns list
- `frontend/src/app/(app)/campaigns/[id]/page.tsx` — single campaign detail
- `frontend/src/app/(app)/leads/page.tsx` — leads table
- `frontend/src/components/app/campaign-card.tsx`
- `frontend/src/components/app/leads-table.tsx`
- `frontend/src/components/app/stats-cards.tsx`

#### Specification

**Design Brief:**
- **Purpose:** Daily operational view. Users check campaign performance and manage leads.
- **Aesthetic:** Data-dense but clean. Tables with good typography, stat cards with big numbers. Similar to Linear's project views.
- **Tone:** Informative, at-a-glance. No clutter.

**Dashboard Page (`/dashboard`):**
- 4 stat cards at top:
  - Total leads discovered (number)
  - Emails sent (number)
  - Replies received (number)
  - Active campaigns (number)
- Recent campaigns list (last 5, with status badge and key metric)
- Quick action: "New Campaign" button → links to onboarding

**Campaigns List (`/campaigns`):**
- Table or card grid:
  - Campaign name
  - Status badge (Draft, Active, Paused, Completed)
  - Play name
  - Leads count
  - Sent / Replied / Bounced counts
  - Created date
- "New Campaign" button in top-right
- Click row → navigate to `/campaigns/[id]`

**Campaign Detail (`/campaigns/[id]`):**
- Top: Campaign name + status + action buttons (Start, Pause, Edit)
- Stat cards: Sent, Replied, Bounced, Interested, Pipeline value
- Tabs:
  - **Leads** — table of leads in this campaign: Name, Title, Company, Email, Status (Queued/Active/Replied/Bounced/Exhausted), Current Step, Last Sent
  - **Sequence** — preview of email sequence steps with delays
- Action: "Add More Leads" button (triggers lead discovery)

**Leads Page (`/leads`):**
- Global leads table (across all campaigns):
  - Name, Title, Company, Email, LinkedIn URL, Source, Added date
- Search/filter bar: search by name, company, or email
- Pagination (20 per page)

**Mock data:**
Use realistic mock data for all views:
- 3 campaigns (1 Active, 1 Draft, 1 Paused)
- 20-30 mock leads with varied statuses
- Realistic company names, titles, emails

#### Acceptance Criteria

- [ ] Dashboard renders at `/dashboard` with 4 stat cards and recent campaigns
- [ ] Campaigns list at `/campaigns` shows table with all columns
- [ ] Campaign detail at `/campaigns/[id]` shows stats, leads tab, and sequence tab
- [ ] Leads page at `/leads` shows searchable, paginated table
- [ ] Status badges have appropriate colors (green=Active, yellow=Paused, gray=Draft)
- [ ] Mobile responsive — tables scroll horizontally, stat cards stack
- [ ] All data is mock (to be replaced in T14)

#### Notes

- Use Shadcn Table component for data tables
- Use Shadcn Badge for status indicators
- The "Pipeline value" stat can be $0 for MVP — it requires deal tracking which is v2
- Consider skeleton loading states for when real data is fetched (T14)

---

### T09: Go HTTP Server + Router + Middleware

**Day:** 4
**Estimated time:** 2 hours
**Depends on:** T01, T02
**Skill/Agent:** Manual / Claude Code

#### Context

Build the Go HTTP server with Chi v5 router, CORS middleware, Clerk JWT verification middleware, request logging, and error handling. This is the backbone that all API endpoints plug into.

#### Inputs

- T01 scaffolding (cmd/api/main.go stub)
- T02 database pool setup
- Clerk Go SDK for JWT verification
- `.env` variables: `CLERK_SECRET_KEY`, `DATABASE_URL`, `REDIS_URL`, `FRONTEND_URL`

#### Deliverables

- `backend/cmd/api/main.go` (full implementation)
- `backend/internal/middleware/auth.go` — Clerk JWT verification
- `backend/internal/middleware/logging.go` — request logging
- `backend/internal/middleware/tenant.go` — tenant context extraction
- `backend/pkg/errors/errors.go` — standard error types and JSON error responses

#### Specification

**`cmd/api/main.go`:**
```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/go-chi/chi/v5"
    chimw "github.com/go-chi/chi/v5/middleware"
    "github.com/go-chi/cors"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/hibiken/asynq"
    "github.com/joho/godotenv"

    "github.com/jcleira/magiklead/backend/internal/handler"
    "github.com/jcleira/magiklead/backend/internal/middleware"
    "github.com/jcleira/magiklead/backend/internal/repository"
    "github.com/jcleira/magiklead/backend/internal/service"
)

func main() {
    godotenv.Load()

    // Database
    pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
    if err != nil {
        log.Fatal("failed to connect to database:", err)
    }
    defer pool.Close()

    // Redis (for Asynq client)
    redisOpt := asynq.RedisClientOpt{Addr: os.Getenv("REDIS_URL")}
    asynqClient := asynq.NewClient(redisOpt)
    defer asynqClient.Close()

    // Repositories
    queries := repository.New(pool)

    // Services
    // (initialized in T10-T13 tasks)

    // Router
    r := chi.NewRouter()

    // Global middleware
    r.Use(chimw.RequestID)
    r.Use(chimw.RealIP)
    r.Use(middleware.Logger)
    r.Use(chimw.Recoverer)
    r.Use(cors.Handler(cors.Options{
        AllowedOrigins:   []string{os.Getenv("FRONTEND_URL")},
        AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
        AllowCredentials: true,
        MaxAge:           300,
    }))

    // Health check
    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("ok"))
    })

    // API routes
    r.Route("/api/v1", func(r chi.Router) {
        // Public routes
        r.Post("/webhooks/clerk", handler.HandleClerkWebhook(queries))
        r.Post("/webhooks/stripe", handler.HandleStripeWebhook(queries))

        // Protected routes
        r.Group(func(r chi.Router) {
            r.Use(middleware.ClerkAuth)
            r.Use(middleware.WithTenant(queries))

            // Routes registered by T10-T13
        })
    })

    // Server
    srv := &http.Server{
        Addr:    ":" + getEnv("PORT", "8080"),
        Handler: r,
    }

    // Graceful shutdown
    go func() {
        log.Printf("Server starting on %s", srv.Addr)
        if err := srv.ListenAndServe(); err != http.ErrServerClosed {
            log.Fatal(err)
        }
    }()

    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}

func getEnv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

**`middleware/auth.go`:**
```go
package middleware

import (
    "context"
    "net/http"

    "github.com/clerk/clerk-sdk-go/v2"
    clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
)

type contextKey string
const UserIDKey contextKey = "userID"

func ClerkAuth(next http.Handler) http.Handler {
    return clerkhttp.WithHeaderAuthorization()(next)
    // After Clerk verification, extract user ID:
    // claims, ok := clerk.SessionClaimsFromContext(r.Context())
}
```

Note: The exact Clerk Go SDK middleware API should be confirmed against the latest clerk-sdk-go v2 docs. The pattern is to use `clerkhttp.WithHeaderAuthorization()` then extract `clerk.SessionClaimsFromContext()`.

**`middleware/tenant.go`:**
```go
package middleware

import (
    "context"
    "net/http"

    "github.com/jcleira/magiklead/backend/internal/repository"
)

const TenantIDKey contextKey = "tenantID"

func WithTenant(queries *repository.Queries) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Get user from Clerk context
            // Look up user's default tenant from user_tenants table
            // Set tenant ID in context
            // For MVP: use X-Tenant-ID header as override
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

**`middleware/logging.go`:**
```go
package middleware

import (
    "log"
    "net/http"
    "time"

    chimw "github.com/go-chi/chi/v5/middleware"
)

func Logger(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
        next.ServeHTTP(ww, r)
        log.Printf("%s %s %d %s", r.Method, r.URL.Path, ww.Status(), time.Since(start))
    })
}
```

**`pkg/errors/errors.go`:**
```go
package errors

import (
    "encoding/json"
    "net/http"
)

type APIError struct {
    Status  int    `json:"-"`
    Code    string `json:"code"`
    Message string `json:"message"`
}

func (e APIError) Error() string { return e.Message }

func WriteError(w http.ResponseWriter, err APIError) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(err.Status)
    json.NewEncoder(w).Encode(err)
}

var (
    ErrNotFound      = APIError{Status: 404, Code: "not_found", Message: "Resource not found"}
    ErrUnauthorized  = APIError{Status: 401, Code: "unauthorized", Message: "Unauthorized"}
    ErrForbidden     = APIError{Status: 403, Code: "forbidden", Message: "Forbidden"}
    ErrBadRequest    = APIError{Status: 400, Code: "bad_request", Message: "Bad request"}
    ErrInternal      = APIError{Status: 500, Code: "internal_error", Message: "Internal server error"}
    ErrQuotaExceeded = APIError{Status: 429, Code: "quota_exceeded", Message: "Plan quota exceeded"}
)
```

#### Acceptance Criteria

- [ ] `go run ./cmd/api/...` starts server on :8080
- [ ] `GET /health` returns 200 "ok"
- [ ] CORS headers present on responses for configured frontend origin
- [ ] Request logging shows method, path, status, duration
- [ ] Unauthenticated requests to protected routes return 401
- [ ] Clerk JWT verification works with valid test token

#### Notes

- For local development, you may need to set `CLERK_SECRET_KEY` to test JWT verification
- The tenant middleware for MVP can use a simple "default tenant" approach — first tenant the user belongs to
- Don't implement route handlers yet — those come in T10-T13

---

### T10: AI Integration (Claude API)

**Day:** 4
**Estimated time:** 3 hours
**Depends on:** T09
**Skill/Agent:** Manual / Claude Code

#### Context

Implement the three core AI functions: website analysis, sales play generation, and email sequence generation. All use Claude Haiku 4.5 via the Anthropic Go SDK. For MVP, we use the synchronous API (not batch) since latency for individual requests is acceptable (2-5 seconds).

The research doc (Section 7) defines the exact input/output shapes for each AI task.

#### Inputs

- Research doc Section 7 (AI Layer — all prompts, input/output shapes, cost estimates)
- Anthropic Go SDK (`github.com/anthropics/anthropic-sdk-go`)
- `ANTHROPIC_API_KEY` from `.env`

#### Deliverables

- `backend/internal/ai/client.go` — shared Claude client setup
- `backend/internal/ai/analyzer.go` — website analysis
- `backend/internal/ai/plays.go` — sales play generation
- `backend/internal/ai/sequences.go` — email sequence generation
- `backend/internal/handler/websites.go` — POST /api/v1/websites/analyze, GET .../profile
- `backend/internal/handler/plays.go` — POST /api/v1/plays/generate, CRUD
- `backend/internal/handler/sequences.go` — POST /api/v1/sequences/generate

#### Specification

**`ai/client.go`:**
```go
package ai

import "github.com/anthropics/anthropic-sdk-go"

type Client struct {
    api *anthropic.Client
}

func NewClient(apiKey string) *Client {
    return &Client{
        api: anthropic.NewClient(anthropic.WithAPIKey(apiKey)),
    }
}
```

**`ai/analyzer.go`:**

Function: `AnalyzeWebsite(ctx context.Context, url string) (*BusinessProfile, error)`

1. Fetch the URL content (HTTP GET, extract text from HTML using a simple HTML-to-text approach)
2. Send to Claude Haiku with system prompt:
   ```
   You are a sales intelligence AI. Analyze the following website content and extract a structured business profile. Return JSON only.
   ```
3. User message: the extracted website text (truncated to 4000 chars)
4. Parse response into `BusinessProfile` struct:
   ```go
   type BusinessProfile struct {
       CompanyName    string   `json:"company_name"`
       Description    string   `json:"product_description"`
       Features       []string `json:"features"`
       Pricing        string   `json:"pricing"`
       TargetCustomers []string `json:"target_customers"`
       Differentiators []string `json:"differentiators"`
       Industry       string   `json:"industry"`
   }
   ```

**`ai/plays.go`:**

Function: `GeneratePlays(ctx context.Context, profile *BusinessProfile) ([]Play, error)`

System prompt:
```
You are a B2B sales strategist. Given a business profile, generate 3-5 sales plays. Each play targets a specific ICP (Ideal Customer Profile) with a specific buying signal and outreach channel. Return JSON array only.
```

Output struct:
```go
type Play struct {
    Name        string      `json:"name"`
    Description string      `json:"description"`
    ICP         ICP         `json:"icp"`
    Signal      string      `json:"signal"`
    Channels    []string    `json:"channels"`
    SearchQuery SearchQuery `json:"search_query"`
}
type ICP struct {
    Titles      []string `json:"titles"`
    Industry    []string `json:"industry"`
    CompanySize string   `json:"company_size"`
    Geo         []string `json:"geo"`
}
type SearchQuery struct {
    Title       string `json:"title"`
    CompanySize string `json:"company_size"`
    Industry    string `json:"industry"`
    Location    string `json:"location"`
}
```

**`ai/sequences.go`:**

Function: `GenerateSequence(ctx context.Context, profile *BusinessProfile, play *Play, lead *LeadInfo) (*Sequence, error)`

System prompt:
```
You are an expert cold email copywriter. Write a 3-4 step email sequence for B2B outreach. Each email should be personalized, concise (under 150 words), and have a clear CTA. Use {{first_name}}, {{company}}, {{title}} as merge variables. Return JSON array only.
```

Output struct:
```go
type Sequence struct {
    Steps []SequenceStep `json:"steps"`
}
type SequenceStep struct {
    Step      int    `json:"step"`
    DelayDays int    `json:"delay_days"`
    Subject   string `json:"subject"`
    Body      string `json:"body"`
}
```

**Handlers:**

`handler/websites.go`:
- `POST /api/v1/websites/analyze` — accepts `{"url": "https://..."}`, calls `AnalyzeWebsite`, stores result in tenant's `business_profile`, returns profile
- `GET /api/v1/websites/profile` — returns tenant's business profile from DB
- `PUT /api/v1/websites/profile` — updates tenant's business profile

`handler/plays.go`:
- `POST /api/v1/plays/generate` — calls `GeneratePlays` with tenant's profile, stores each play, returns all
- `GET /api/v1/plays` — list plays for tenant
- `GET /api/v1/plays/:id` — get single play
- `PUT /api/v1/plays/:id` — update play
- `DELETE /api/v1/plays/:id` — delete play

`handler/sequences.go`:
- `POST /api/v1/sequences/generate` — accepts `{"play_id": "...", "lead_ids": [...]}`, generates sequences for each lead, returns them

#### Acceptance Criteria

- [ ] `POST /api/v1/websites/analyze` with a valid URL returns a structured BusinessProfile JSON
- [ ] `POST /api/v1/plays/generate` returns 3-5 plays with realistic ICPs and search queries
- [ ] `POST /api/v1/sequences/generate` returns a 3-4 step email sequence with personalization variables
- [ ] Plays are persisted in the database and retrievable via GET
- [ ] AI errors (rate limit, invalid response) return appropriate HTTP error codes
- [ ] Website content is truncated to prevent token overuse

#### Notes

- Use `claude-haiku-4-5-20251001` model ID for all requests
- Set `max_tokens: 2048` for all requests — sufficient for all output shapes
- The website scraper doesn't need to be sophisticated — `net/http` GET + basic HTML tag stripping is enough for MVP
- Consider adding a 30-second timeout for AI calls
- Store the raw AI response in a `raw_data` JSONB field for debugging

---

### T11: Lead Discovery Pipeline

**Day:** 4
**Estimated time:** 4 hours
**Depends on:** T09, T02
**Skill/Agent:** Manual / Claude Code

#### Context

Implement the lead discovery pipeline from the research doc's Section 4: LinkedIn people search via RapidAPI → email finding → email verification → cache in PostgreSQL. This is the core of the zero-marginal-cost strategy.

The pipeline runs asynchronously via Asynq. When a user triggers lead discovery for a play, an Asynq task is enqueued. The worker processes the search, stores leads in the DB, and updates the campaign.

#### Inputs

- Research doc Section 4 (Lead Discovery Pipeline — full data flow, API providers, caching strategy)
- RapidAPI providers:
  - LinkedIn: `fresh-linkedin-profile-data` ($0.002/request)
  - Email: `email-search16` ($0.002/request)
  - Verification: `myemailverifier1` ($0.0025/request)
- `RAPIDAPI_KEY` from `.env`
- T02 database schema (leads, lead_searches tables)

#### Deliverables

- `backend/internal/leads/linkedin.go` — RapidAPI LinkedIn search client
- `backend/internal/leads/email.go` — RapidAPI email finder client
- `backend/internal/leads/verify.go` — email verification client
- `backend/internal/leads/pipeline.go` — orchestrates the full discovery flow
- `backend/internal/leads/cache.go` — lead search caching logic
- `backend/internal/worker/discover.go` — Asynq task handler for lead discovery
- `backend/internal/handler/leads.go` — lead API endpoints

#### Specification

**`leads/linkedin.go`:**
```go
package leads

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
)

const linkedinAPIURL = "https://fresh-linkedin-profile-data.p.rapidapi.com"

type LinkedInClient struct {
    apiKey string
    http   *http.Client
}

type LinkedInSearchResult struct {
    Profiles []LinkedInProfile `json:"data"`
    Total    int               `json:"total"`
}

type LinkedInProfile struct {
    FullName     string `json:"full_name"`
    FirstName    string `json:"first_name"`
    LastName     string `json:"last_name"`
    Title        string `json:"title"`
    Company      string `json:"company"`
    Location     string `json:"location"`
    ProfileURL   string `json:"linkedin_url"`
    Summary      string `json:"summary"`
}

func (c *LinkedInClient) SearchPeople(ctx context.Context, query SearchQuery) (*LinkedInSearchResult, error) {
    // GET /search-people?keywords=...&title=...&company_size=...&location=...
    // Header: X-RapidAPI-Key, X-RapidAPI-Host
    // Returns paginated results (10 per page)
}
```

**`leads/email.go`:**
```go
func (c *EmailClient) FindEmail(ctx context.Context, firstName, lastName, domain string) (string, error) {
    // POST to email-search16 API
    // Input: first_name, last_name, domain
    // Output: email address string
}
```

**`leads/verify.go`:**
```go
func (c *VerifyClient) VerifyEmail(ctx context.Context, email string) (bool, error) {
    // POST to myemailverifier1 API
    // Returns: verified boolean
}
```

**`leads/pipeline.go`:**
```go
func (p *Pipeline) DiscoverLeads(ctx context.Context, query SearchQuery, tenantID uuid.UUID) ([]Lead, error) {
    // 1. Check cache (lead_searches by query hash)
    // 2. If cache hit and fresh (< TTL days): return cached leads
    // 3. If cache miss:
    //    a. Search LinkedIn API
    //    b. For each profile:
    //       - Find email (email search API)
    //       - Verify email
    //       - Upsert lead in DB (ON CONFLICT linkedin_url DO UPDATE)
    //    c. Store search result in lead_searches table
    //    d. Return leads
}

func hashQuery(query SearchQuery) string {
    // SHA256 of normalized, sorted JSON of search params
}
```

**`leads/cache.go`:**
```go
func (c *Cache) Get(ctx context.Context, queryHash string) ([]uuid.UUID, error) {
    // SELECT from lead_searches WHERE query_hash = $1 AND fresh
}

func (c *Cache) Set(ctx context.Context, queryHash string, filters json.RawMessage, leadIDs []uuid.UUID) error {
    // INSERT into lead_searches
}
```

**`worker/discover.go`:**
```go
const TypeDiscoverLeads = "discover:leads"

type DiscoverLeadsPayload struct {
    TenantID   uuid.UUID   `json:"tenant_id"`
    PlayID     uuid.UUID   `json:"play_id"`
    CampaignID uuid.UUID   `json:"campaign_id"`
    Query      SearchQuery `json:"query"`
}

func HandleDiscoverLeads(pipeline *leads.Pipeline, queries *repository.Queries) asynq.HandlerFunc {
    return func(ctx context.Context, t *asynq.Task) error {
        // 1. Unmarshal payload
        // 2. Call pipeline.DiscoverLeads()
        // 3. For each lead: insert campaign_lead record
        // 4. Update campaign stats
        // 5. Update subscription leads_used count
    }
}
```

**`handler/leads.go`:**
- `GET /api/v1/leads` — list leads for tenant (across all campaigns), with filters
- `POST /api/v1/leads/discover` — enqueue discovery task, accepts `{"play_id": "...", "campaign_id": "..."}`
- `GET /api/v1/leads/:id` — get single lead

#### Acceptance Criteria

- [ ] `POST /api/v1/leads/discover` enqueues an Asynq task and returns 202 with task ID
- [ ] Worker processes the task: searches LinkedIn, finds emails, verifies, stores in DB
- [ ] Subsequent identical searches return cached results (no API calls)
- [ ] Leads are upserted by linkedin_url (no duplicates)
- [ ] `GET /api/v1/leads` returns discovered leads for the tenant
- [ ] RapidAPI errors are handled gracefully (retry on 429, fail on 4xx/5xx)
- [ ] Subscription leads_used is incremented after discovery

#### Notes

- Start with RapidAPI free tier (50 requests/month) for development
- The search query hash must be deterministic — normalize and sort JSON keys before hashing
- Add a 1-second delay between RapidAPI calls to respect rate limits
- Log the RapidAPI response body for debugging during development
- For leads where email finding fails, still store the lead (email = NULL) — they can be enriched later

---

### T12: Campaign Engine + Asynq Workers

**Day:** 4
**Estimated time:** 3 hours
**Depends on:** T09, T10, T11
**Skill/Agent:** Manual / Claude Code

#### Context

Build the campaign management system and the Asynq worker that processes email sequence execution. A campaign ties together a play, a set of leads, and an email sequence. The worker checks for leads that are due for their next email step and sends them via Gmail (T13).

#### Inputs

- Research doc Section 5 (Email Sequence Engine — step processing logic)
- Research doc Section 12 (API Design — campaign endpoints)
- T02 database schema (campaigns, campaign_leads tables)
- T10 AI sequence generation
- T11 lead discovery

#### Deliverables

- `backend/internal/handler/campaigns.go` — campaign CRUD + start/pause
- `backend/internal/service/campaign.go` — campaign business logic
- `backend/internal/worker/sequence.go` — Asynq periodic task: process due leads
- `backend/cmd/worker/main.go` (full implementation)

#### Specification

**`handler/campaigns.go`:**
```
POST   /api/v1/campaigns           — create campaign (name, play_id, gmail_account_id)
GET    /api/v1/campaigns           — list campaigns for tenant
GET    /api/v1/campaigns/:id       — get campaign detail with lead counts by status
PUT    /api/v1/campaigns/:id       — update campaign name/settings
POST   /api/v1/campaigns/:id/start — set status=active, set all queued leads to active with next_send_at
POST   /api/v1/campaigns/:id/pause — set status=paused, nullify next_send_at for active leads
POST   /api/v1/campaigns/:id/leads — add lead IDs to campaign
```

**`service/campaign.go`:**
```go
type CampaignService struct {
    queries    *repository.Queries
    aiClient   *ai.Client
    asynqClient *asynq.Client
}

func (s *CampaignService) Create(ctx context.Context, tenantID uuid.UUID, req CreateCampaignRequest) (*Campaign, error) {
    // 1. Get play from DB
    // 2. Get tenant profile
    // 3. Generate email sequence via AI (if not provided)
    // 4. Create campaign record
    // 5. Return campaign
}

func (s *CampaignService) Start(ctx context.Context, campaignID, tenantID uuid.UUID) error {
    // 1. Update campaign status to 'active'
    // 2. For all campaign_leads with status='queued':
    //    - Set status='active'
    //    - Set next_send_at = NOW() (first email sends immediately)
}

func (s *CampaignService) Pause(ctx context.Context, campaignID, tenantID uuid.UUID) error {
    // 1. Update campaign status to 'paused'
    // 2. Set all active campaign_leads next_send_at = NULL
}
```

**`worker/sequence.go`:**
```go
const TypeProcessSequence = "sequence:process"

// This runs as a periodic task every 60 seconds
func HandleProcessSequence(queries *repository.Queries, gmailSvc *gmail.Service) asynq.HandlerFunc {
    return func(ctx context.Context, t *asynq.Task) error {
        // 1. GetDueLeads(limit=50) — campaign_leads where next_send_at <= NOW() AND status='active'
        // 2. For each due lead:
        //    a. Get campaign (for sequence JSON)
        //    b. Get current step from sequence
        //    c. If no more steps: mark as 'exhausted'
        //    d. Send email via Gmail API (T13)
        //    e. Create email_event (type='sent')
        //    f. Update campaign_lead: increment step, set next_send_at = NOW() + step.delay_days
        //    g. Update campaign stats
    }
}
```

**`cmd/worker/main.go`:**
```go
func main() {
    godotenv.Load()

    pool, _ := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
    defer pool.Close()
    queries := repository.New(pool)

    // Services
    gmailSvc := gmail.NewService(...)
    // ...

    srv := asynq.NewServer(
        asynq.RedisClientOpt{Addr: os.Getenv("REDIS_URL")},
        asynq.Config{Concurrency: 10},
    )

    mux := asynq.NewServeMux()
    mux.HandleFunc(TypeDiscoverLeads, HandleDiscoverLeads(pipeline, queries))
    mux.HandleFunc(TypeProcessSequence, HandleProcessSequence(queries, gmailSvc))

    // Periodic tasks
    scheduler := asynq.NewScheduler(
        asynq.RedisClientOpt{Addr: os.Getenv("REDIS_URL")},
        nil,
    )
    scheduler.Register("@every 60s", asynq.NewTask(TypeProcessSequence, nil))

    go scheduler.Run()
    srv.Run(mux)
}
```

#### Acceptance Criteria

- [ ] `POST /api/v1/campaigns` creates a campaign with AI-generated sequence
- [ ] `POST /api/v1/campaigns/:id/start` activates the campaign and queues leads for sending
- [ ] `POST /api/v1/campaigns/:id/pause` pauses sending
- [ ] Worker processes due leads every 60 seconds
- [ ] Each lead progresses through sequence steps with correct delays
- [ ] Leads are marked 'exhausted' after the last step
- [ ] Campaign stats (sent count) are updated after each send
- [ ] `GET /api/v1/campaigns/:id` returns accurate lead counts by status

#### Notes

- The worker should process at most 50 leads per cycle to avoid Gmail rate limits
- If Gmail sending fails for a lead, don't advance the step — retry next cycle
- Log every send for debugging: campaign_id, lead_id, step, email address
- The periodic task should acquire a distributed lock (Redis SETNX) to prevent duplicate processing in multi-worker setups

---

### T13: Gmail OAuth + Email Sending

**Day:** 5
**Estimated time:** 3 hours
**Depends on:** T09, T02
**Skill/Agent:** Manual / Claude Code

#### Context

Implement Gmail OAuth 2.0 integration so users can connect their Google account and send emails through the Gmail API. The research doc (Section 5) specifies the scopes, limits, and approach.

This is a critical path item — without Gmail sending, the product can't execute outreach.

#### Inputs

- Research doc Section 5 (Email Infrastructure — OAuth scopes, sending approach, limits)
- Google OAuth 2.0 docs
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI` from `.env`
- T02 database schema (gmail_accounts table)

#### Deliverables

- `backend/internal/gmail/oauth.go` — OAuth flow (auth URL, callback, token storage)
- `backend/internal/gmail/send.go` — send email via Gmail API
- `backend/internal/gmail/service.go` — service layer tying it together
- `backend/internal/handler/gmail.go` — Gmail API endpoints

#### Specification

**`gmail/oauth.go`:**
```go
package gmail

import (
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
)

func NewOAuthConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
    return &oauth2.Config{
        ClientID:     clientID,
        ClientSecret: clientSecret,
        RedirectURL:  redirectURI,
        Scopes: []string{
            "https://www.googleapis.com/auth/gmail.send",
            "https://www.googleapis.com/auth/gmail.readonly",
            "https://www.googleapis.com/auth/userinfo.email",
        },
        Endpoint: google.Endpoint,
    }
}

func (s *Service) GetAuthURL(state string) string {
    return s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

func (s *Service) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
    return s.oauthConfig.Exchange(ctx, code)
}
```

**`gmail/send.go`:**
```go
func (s *Service) SendEmail(ctx context.Context, accountID uuid.UUID, to, subject, body, senderName string) error {
    // 1. Get gmail_account from DB
    // 2. Create oauth2 token from stored access/refresh tokens
    // 3. Create Gmail API service with token source
    // 4. Build RFC 2822 message:
    //    From: "Sender Name" <sender@gmail.com>
    //    To: recipient@company.com
    //    Subject: subject
    //    Content-Type: text/html; charset=utf-8
    //    
    //    body (with unsubscribe footer)
    // 5. Base64url encode the message
    // 6. Call gmail.Users.Messages.Send("me", &gmail.Message{Raw: encoded})
    // 7. Increment daily_sent_count
    // 8. If token refreshed, update stored tokens
}

func buildMessage(from, fromName, to, subject, htmlBody string) []byte {
    // Build RFC 2822 compliant email
    // Include CAN-SPAM footer with unsubscribe link
}
```

**CAN-SPAM footer** (automatically appended to every email):
```html
<div style="margin-top: 20px; padding-top: 10px; border-top: 1px solid #eee; font-size: 11px; color: #999;">
  <p>Sent via MagikLead. <a href="{{unsubscribe_url}}">Unsubscribe</a></p>
</div>
```

**`handler/gmail.go`:**
```
GET    /api/v1/gmail/auth-url      — returns OAuth authorization URL
GET    /api/v1/gmail/callback      — handles OAuth callback, stores tokens
GET    /api/v1/gmail/accounts      — list connected Gmail accounts
DELETE /api/v1/gmail/accounts/:id  — disconnect (delete tokens)
```

The callback flow:
1. Frontend redirects user to auth URL
2. Google redirects back to callback URL with code
3. Backend exchanges code for tokens
4. Stores tokens in gmail_accounts table
5. Redirects user back to frontend settings page

#### Acceptance Criteria

- [ ] `GET /api/v1/gmail/auth-url` returns a valid Google OAuth URL
- [ ] OAuth callback exchanges code and stores tokens in DB
- [ ] `SendEmail()` sends an email via Gmail API that arrives in recipient's inbox
- [ ] Every sent email includes CAN-SPAM compliant unsubscribe footer
- [ ] Token refresh works transparently when access token expires
- [ ] Daily send count is tracked and enforced (< 2000/day)
- [ ] `GET /api/v1/gmail/accounts` lists connected accounts

#### Notes

- Google OAuth requires HTTPS in production — use the Vercel proxy or Caddy for local testing
- For development, you can use Google's OAuth playground or a test project
- Store tokens encrypted at rest (use AES-GCM with a key from env)
- The unsubscribe URL should point to a backend endpoint that marks the campaign_lead as 'unsubscribed'
- Gmail API returns message ID — store it for threading replies later (Phase 2)

---

### T14: Frontend-Backend Integration

**Day:** 5
**Estimated time:** 3 hours
**Depends on:** T07, T08, T09, T10, T11, T12, T13
**Skill/Agent:** Manual / Claude Code

#### Context

Wire the frontend to the real backend API. Replace all mock data with API calls. Add proper loading states, error states, and token passing (Clerk JWT → Authorization header).

#### Inputs

- T07 onboarding UI (mock data)
- T08 dashboard UI (mock data)
- T09 Go server running on :8080
- API endpoints from T10-T13
- Clerk Next.js SDK for auth tokens

#### Deliverables

- `frontend/src/lib/api.ts` — full API client with auth
- `frontend/src/hooks/use-campaigns.ts` — campaign data hook
- `frontend/src/hooks/use-leads.ts` — leads data hook
- `frontend/src/hooks/use-tenant.ts` — tenant/subscription hook
- Updated onboarding pages (real API calls)
- Updated dashboard/campaign pages (real API calls)

#### Specification

**`lib/api.ts`** (updated with Clerk auth):
```typescript
import { auth } from "@clerk/nextjs/server";

export async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
  const { getToken } = await auth();
  const token = await getToken();

  const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${token}`,
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const error = await res.json().catch(() => ({ message: "Unknown error" }));
    throw new Error(error.message || `API error: ${res.status}`);
  }

  return res.json();
}
```

For client components, use `useAuth()` from `@clerk/nextjs`:
```typescript
"use client";
import { useAuth } from "@clerk/nextjs";

export function useApiClient() {
  const { getToken } = useAuth();

  return async function apiFetch<T>(path: string, options?: RequestInit): Promise<T> {
    const token = await getToken();
    // ... same as above but client-side
  };
}
```

**Integration points:**

| UI Component | API Call | Loading State | Error State |
|-------------|---------|---------------|-------------|
| Onboarding Step 1 | POST /api/v1/websites/analyze | "Analyzing..." spinner | "Failed to analyze. Try again." |
| Onboarding Step 3 | POST /api/v1/plays/generate | "Generating plays..." | "Failed to generate." |
| Onboarding Step 4 | POST /api/v1/campaigns + POST .../leads/discover | "Discovering leads..." | Error toast |
| Dashboard stats | GET /api/v1/analytics/overview | Skeleton cards | Fallback to zeros |
| Campaigns list | GET /api/v1/campaigns | Skeleton rows | "No campaigns yet" empty state |
| Campaign detail | GET /api/v1/campaigns/:id | Full page skeleton | 404 page |
| Leads table | GET /api/v1/leads | Skeleton table | "No leads yet" empty state |
| Gmail connect | GET /api/v1/gmail/auth-url → redirect | Redirect loading | Error toast |

#### Acceptance Criteria

- [ ] Onboarding flow calls real API: paste URL → analyze → generate plays → create campaign
- [ ] Dashboard shows real data from API
- [ ] Campaigns list shows campaigns from API
- [ ] Campaign detail shows real leads and stats
- [ ] Leads table shows real leads with search/pagination
- [ ] Loading states show Shadcn Skeleton components
- [ ] Error states show user-friendly messages
- [ ] Auth token is included in all API requests
- [ ] No mock data remains in production code (mock can stay as fallback for dev)

#### Notes

- Use SWR or React Query patterns for data fetching — or just `useEffect` + `useState` for MVP simplicity
- Add a global error boundary in the app layout
- Clerk tokens have a TTL — use `getToken()` fresh for each request, don't cache

---

### T15: Stripe Integration

**Day:** 5
**Estimated time:** 3 hours
**Depends on:** T09, T02
**Skill/Agent:** Manual / Claude Code

#### Context

Integrate Stripe for subscription billing. The research doc (Section 10) defines 4 tiers. We need: Stripe Products + Prices, checkout flow, webhook handlers for subscription lifecycle, and customer portal for self-service management.

#### Inputs

- Research doc Section 10 (Pricing Model — tiers, prices, features)
- Stripe Go SDK (`github.com/stripe/stripe-go/v81`)
- `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` from `.env`
- T02 database schema (subscriptions table)

#### Deliverables

- `backend/internal/handler/stripe.go` — webhook handler
- `backend/internal/handler/billing.go` — checkout, portal, subscription endpoints
- `backend/internal/service/billing.go` — billing business logic
- `frontend/src/app/(app)/settings/page.tsx` — settings page with billing section (plan display, upgrade button)

#### Specification

**Stripe Products to Create (manually in Stripe Dashboard or via API):**

| Product | Price ID Pattern | Monthly | Annual |
|---------|-----------------|---------|--------|
| Starter | price_starter_monthly / price_starter_annual | $49 | $468 ($39/mo) |
| Growth | price_growth_monthly / price_growth_annual | $149 | $1,428 ($119/mo) |
| Scale | price_scale_monthly / price_scale_annual | $399 | $3,828 ($319/mo) |

Store price IDs as environment variables:
```
STRIPE_PRICE_STARTER_MONTHLY=price_xxx
STRIPE_PRICE_STARTER_ANNUAL=price_xxx
STRIPE_PRICE_GROWTH_MONTHLY=price_xxx
STRIPE_PRICE_GROWTH_ANNUAL=price_xxx
STRIPE_PRICE_SCALE_MONTHLY=price_xxx
STRIPE_PRICE_SCALE_ANNUAL=price_xxx
```

**`handler/billing.go`:**
```
POST /api/v1/billing/checkout    — create Stripe Checkout Session, return URL
POST /api/v1/billing/portal      — create Stripe Customer Portal Session, return URL
GET  /api/v1/billing/subscription — get current subscription details
```

Checkout flow:
1. Frontend calls POST /api/v1/billing/checkout with `{"price_id": "price_xxx"}`
2. Backend creates Stripe Checkout Session with `mode: "subscription"`
3. Returns checkout URL
4. Frontend redirects to Stripe Checkout
5. After payment, Stripe redirects back to `/settings?checkout=success`
6. Webhook handles `checkout.session.completed`

**`handler/stripe.go` (webhook):**

Handle these events:
- `checkout.session.completed` → create/update subscription in DB
- `customer.subscription.updated` → update plan, limits, period
- `customer.subscription.deleted` → downgrade to free tier
- `invoice.payment_failed` → flag subscription (don't immediately downgrade)

```go
func HandleStripeWebhook(queries *repository.Queries) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Read body
        // 2. Verify webhook signature
        // 3. Parse event
        // 4. Switch on event type
        // 5. Process and update DB
        w.WriteHeader(200)
    }
}
```

**Plan limits mapping:**
```go
var PlanLimits = map[string]struct {
    Leads     int
    Sequences int
}{
    "free":    {100, 300},
    "starter": {500, 1500},
    "growth":  {2000, 6000},
    "scale":   {10000, 30000},
}
```

**Settings page** (`frontend/src/app/(app)/settings/page.tsx`):
- Current plan display (plan name, limits, usage)
- "Upgrade" button → redirects to Stripe Checkout
- "Manage Billing" button → redirects to Stripe Customer Portal
- Gmail accounts list with connect/disconnect

#### Acceptance Criteria

- [ ] POST /api/v1/billing/checkout returns a valid Stripe Checkout URL
- [ ] Completing checkout creates a subscription record in DB with correct plan and limits
- [ ] Customer portal allows plan changes and cancellation
- [ ] Webhook handles subscription creation, update, and deletion
- [ ] Webhook signature verification works
- [ ] Settings page shows current plan and usage
- [ ] Free tier users see upgrade prompts
- [ ] Subscription period dates are tracked for usage reset

#### Notes

- Use Stripe test mode for development
- Create a `STRIPE_WEBHOOK_SECRET` via Stripe CLI: `stripe listen --forward-to localhost:8080/api/v1/webhooks/stripe`
- The free tier has no Stripe subscription — it's the default state
- Don't block users immediately on limit exceeded — show a warning, enforce on the next action

---

### T16: Auth Flows (Clerk Webhooks + User Setup)

**Day:** 5
**Estimated time:** 2 hours
**Depends on:** T09, T02
**Skill/Agent:** Manual / Claude Code

#### Context

Handle the Clerk → Backend user synchronization. When a user signs up via Clerk, a webhook creates their user record in our database, creates a default tenant, and assigns the free plan. This ensures every authenticated user has a tenant and subscription ready to go.

#### Inputs

- Clerk webhook docs
- `CLERK_WEBHOOK_SECRET` from `.env`
- T02 database schema (users, user_tenants, tenants, subscriptions tables)

#### Deliverables

- `backend/internal/handler/clerk.go` — Clerk webhook handler
- Updated `frontend/src/middleware.ts` — redirect new users to onboarding

#### Specification

**`handler/clerk.go`:**

Handle `user.created` event:
```go
func HandleClerkWebhook(queries *repository.Queries) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Verify Svix webhook signature
        // 2. Parse event payload
        // 3. Switch on event type:

        // user.created:
        //   a. Create user record (clerk_id, email, name)
        //   b. Create default tenant (name = user's name + "'s Workspace", domain = "")
        //   c. Create user_tenant link (role = 'owner')
        //   d. Create free subscription (leads_limit = 100, sequences_limit = 300)

        // user.updated:
        //   a. Update user email/name if changed

        // user.deleted:
        //   a. Soft delete or mark inactive (don't delete data)

        w.WriteHeader(200)
    }
}
```

**Frontend redirect logic:**

In the app layout, check if the user has completed onboarding (has a tenant with a business_profile). If not, redirect to `/onboarding`.

```typescript
// In (app)/layout.tsx or a provider:
// Fetch user's tenant on mount
// If tenant.business_profile is null → redirect to /onboarding
// Otherwise → render children
```

#### Acceptance Criteria

- [ ] Clerk webhook `user.created` creates user + tenant + free subscription in DB
- [ ] New users are redirected to `/onboarding` on first visit
- [ ] Users with completed onboarding go to `/dashboard`
- [ ] Webhook signature verification rejects invalid payloads
- [ ] Multiple webhook deliveries are idempotent (don't create duplicate users)

#### Notes

- Use Clerk's Svix library for webhook verification
- The webhook endpoint must be public (not behind auth middleware) — it's already routed before the auth middleware in T09
- For local testing: use Clerk Dashboard to trigger test webhooks, or ngrok
- Idempotency: check if user with clerk_id already exists before creating

---

### T17: SEO Comparison Pages + Content

**Day:** 6
**Estimated time:** 3 hours
**Depends on:** T04
**Skill/Agent:** Claude Code + Manual

#### Context

Build the SEO-driven marketing pages that target high-intent comparison keywords. The research doc's customer acquisition strategy (Section 15) identifies comparison pages as P0 priority for organic traffic.

These pages are static content (Server Components) optimized for Google ranking. They target users actively searching for alternatives to competitors.

#### Inputs

- Research doc Section 15 (Customer Acquisition Strategy — keyword targets, content pages)
- Research doc Section 2 (Competitive Analysis — competitor pricing and features)
- Research doc Section 16 (Differentiation Strategy — positioning, switching reasons)
- T04 design tokens and marketing layout

#### Deliverables

- `frontend/src/app/(marketing)/vs/apollo/page.tsx` — MagikLead vs Apollo
- `frontend/src/app/(marketing)/vs/instantly/page.tsx` — MagikLead vs Instantly
- `frontend/src/app/(marketing)/vs/lemlist/page.tsx` — MagikLead vs Lemlist
- `frontend/src/app/(marketing)/best-cold-email-tools/page.tsx` — listicle
- `frontend/src/components/marketing/comparison-table.tsx` — reusable comparison component

#### Specification

**Design Brief:**
- **Purpose:** Rank on Google for "[competitor] alternative" and "best cold email tool" queries
- **Aesthetic:** Clean, content-focused. Same design tokens as landing page. Lots of whitespace, clear headings, comparison tables.
- **Tone:** Fair, factual, confident. Don't trash competitors — acknowledge their strengths, explain where MagikLead wins.

**"MagikLead vs [Competitor]" Page Template:**

1. **H1:** "MagikLead vs [Competitor]: Which Is Better for Cold Outreach in 2026?"
2. **TL;DR box:** 2-3 sentence summary of the comparison
3. **Side-by-side comparison table:**

| Feature | MagikLead | [Competitor] |
|---------|-----------|-------------|
| Starting price | $49/mo | $X/mo |
| Email outreach | Yes | Yes/No |
| LinkedIn outreach | Yes (coming soon) | Yes/No |
| AI sequences | Claude AI | [Their AI] |
| Lead discovery | Built-in (cached) | [Their approach] |
| Email accounts | Bring your Gmail | [Their approach] |
| ... | ... | ... |

4. **Detailed feature breakdown** — 3-4 sections comparing specific features
5. **Pricing comparison** — what you'd pay for equivalent functionality
6. **"Who should choose MagikLead"** — target persona
7. **"Who should choose [Competitor]"** — honest assessment of when they're better
8. **CTA:** "Try MagikLead Free"

**"Best Cold Email Tools 2026" Listicle:**

1. **H1:** "7 Best Cold Email Tools in 2026 (Tested and Compared)"
2. **Intro:** What we tested and how we scored
3. **Rankings:** (MagikLead at #2-3, not #1 — credibility)
   - #1: Apollo (best database)
   - #2: MagikLead (best value)
   - #3: Instantly (best deliverability)
   - #4: Lemlist (best personalization)
   - #5: Smartlead (best for agencies)
   - #6: Waalaxy (best for LinkedIn-only)
   - #7: Saleshandy (cheapest)
4. Each entry: screenshot, pricing, pros, cons, verdict
5. **Comparison table** with all tools
6. **CTA:** "Try MagikLead Free"

**SEO metadata for each page:**
```typescript
export const metadata: Metadata = {
  title: "MagikLead vs Apollo: Complete Comparison 2026",
  description: "Compare MagikLead and Apollo.io for cold email outreach. See pricing, features, and which is better for startups in 2026.",
  openGraph: { ... },
};
```

#### Acceptance Criteria

- [ ] 3 comparison pages render at `/vs/apollo`, `/vs/instantly`, `/vs/lemlist`
- [ ] Listicle renders at `/best-cold-email-tools`
- [ ] All pages have correct metadata (title, description, OG tags)
- [ ] Comparison tables show accurate data from research doc
- [ ] Pages are Server Components (no client JS needed)
- [ ] Mobile responsive
- [ ] Internal links between comparison pages and pricing page
- [ ] CTA buttons link to Clerk sign-up

#### Notes

- These pages are mostly static content — keep them as Server Components for SEO
- The comparison data should be extracted into a shared config/data file, not hardcoded in each page
- Don't make these read like ads — Google penalizes thin affiliate-style content. Each page should be genuinely useful.

---

### T18: Legal Pages + Technical SEO

**Day:** 6
**Estimated time:** 3 hours
**Depends on:** T04
**Skill/Agent:** Manual / Claude Code

#### Context

Create the legally required pages (Privacy Policy, Terms of Service, Refund Policy) and implement technical SEO (sitemap, robots.txt, schema markup). The research doc's legal section (Section 18) identifies specific clauses needed for a cold email tool.

#### Inputs

- Research doc Section 18 (Legal & Compliance — specific risks and required pages)
- Research doc Section 15 (Customer Acquisition — SEO technical requirements)
- T04 marketing layout

#### Deliverables

- `frontend/src/app/(marketing)/privacy/page.tsx`
- `frontend/src/app/(marketing)/terms/page.tsx`
- `frontend/src/app/(marketing)/refund/page.tsx`
- `frontend/src/app/sitemap.ts` — dynamic sitemap
- `frontend/src/app/robots.ts` — robots.txt
- `frontend/src/app/layout.tsx` — updated with schema.org markup

#### Specification

**Privacy Policy** must include:
- What data we collect (lead data from RapidAPI, Gmail OAuth tokens, Clerk user data)
- How we use it (to provide the service, send emails on user's behalf)
- Third-party sharing (RapidAPI for leads, Anthropic for AI, Stripe for billing, Clerk for auth)
- GDPR rights (access, correction, deletion, portability, objection)
- CCPA rights (right to know, delete, opt-out)
- Cookie usage (minimal — Clerk session only)
- Data retention (leads cached indefinitely, user data until account deletion)
- Contact information for data requests

**Terms of Service** must include:
- Acceptable use policy (no spam, comply with CAN-SPAM, no illegal content)
- Account responsibility (users responsible for their outreach content)
- Service limitations (Gmail API limits, rate limits)
- Liability limitation
- Arbitration clause
- Termination conditions
- Intellectual property

**Refund Policy:**
- 7-day refund window from subscription start
- No refund after lead credits consumed
- Cancellation takes effect at end of billing period

**Sitemap** (`src/app/sitemap.ts`):
```typescript
import { MetadataRoute } from "next";

export default function sitemap(): MetadataRoute.Sitemap {
  const baseUrl = "https://magiklead.com";
  return [
    { url: baseUrl, lastModified: new Date(), priority: 1.0 },
    { url: `${baseUrl}/pricing`, lastModified: new Date(), priority: 0.9 },
    { url: `${baseUrl}/vs/apollo`, lastModified: new Date(), priority: 0.8 },
    { url: `${baseUrl}/vs/instantly`, lastModified: new Date(), priority: 0.8 },
    { url: `${baseUrl}/vs/lemlist`, lastModified: new Date(), priority: 0.8 },
    { url: `${baseUrl}/best-cold-email-tools`, lastModified: new Date(), priority: 0.8 },
    { url: `${baseUrl}/privacy`, lastModified: new Date(), priority: 0.3 },
    { url: `${baseUrl}/terms`, lastModified: new Date(), priority: 0.3 },
  ];
}
```

**Robots.txt** (`src/app/robots.ts`):
```typescript
import { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: { userAgent: "*", allow: "/", disallow: ["/dashboard", "/campaigns", "/settings", "/onboarding"] },
    sitemap: "https://magiklead.com/sitemap.xml",
  };
}
```

**Schema.org markup** in root layout:
```html
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  "name": "MagikLead",
  "description": "AI-powered cold email and outreach automation platform",
  "applicationCategory": "BusinessApplication",
  "offers": {
    "@type": "Offer",
    "price": "0",
    "priceCurrency": "USD"
  }
}
</script>
```

#### Acceptance Criteria

- [ ] Privacy Policy, Terms, and Refund pages render and are linked from footer
- [ ] Privacy Policy includes GDPR and CCPA sections
- [ ] Terms include acceptable use policy for cold email
- [ ] `/sitemap.xml` returns valid sitemap with all public pages
- [ ] `/robots.txt` disallows app routes, allows marketing routes
- [ ] Schema.org JSON-LD is present in the page source
- [ ] All marketing pages have unique meta titles and descriptions

#### Notes

- Legal pages are not a substitute for actual legal review — flag this for later
- Use plain, readable language — not legalese
- Include a "Last updated: [date]" at the top of each legal page
- The unsubscribe mechanism mentioned in CAN-SPAM compliance (T13) should be referenced in the Privacy Policy

---

### T19: Production Deployment

**Day:** 7
**Estimated time:** 3 hours
**Depends on:** T01-T18
**Skill/Agent:** Manual / Claude Code

#### Context

Deploy the full stack to production: Go backend + Asynq worker on Hetzner VPS (via Docker Compose + Caddy), Next.js frontend on Vercel, PostgreSQL on Neon (managed). DNS pointing to the correct servers.

#### Inputs

- All previous tasks complete
- Domain registered: magiklead.com
- Hetzner VPS provisioned (CX22: 2 vCPU, 4GB RAM, ~$5/mo)
- Neon database created
- Vercel project linked to repo
- Stripe products created in live mode
- Clerk production instance configured

#### Deliverables

- Backend running on Hetzner at `api.magiklead.com`
- Frontend running on Vercel at `magiklead.com`
- Database running on Neon
- SSL via Caddy (automatic Let's Encrypt)
- All environment variables set in production

#### Specification

**Hetzner Setup:**
```bash
# SSH into server
ssh root@<hetzner-ip>

# Install Docker
curl -fsSL https://get.docker.com | sh

# Clone repo
git clone https://github.com/jcleira/magiklead.git
cd magiklead

# Create .env with production values
cat > .env << 'EOF'
DATABASE_URL=postgres://...@...neon.tech/magiklead?sslmode=require
REDIS_URL=localhost:6379
CLERK_SECRET_KEY=sk_live_...
ANTHROPIC_API_KEY=sk-ant-...
RAPIDAPI_KEY=...
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URI=https://api.magiklead.com/api/v1/gmail/callback
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...
FRONTEND_URL=https://magiklead.com
PORT=8080
EOF

# Run migrations against Neon
DATABASE_URL="postgres://..." go run ./backend/cmd/migrate/...

# Start services
docker compose -f docker-compose.prod.yml up -d --build
```

**DNS Configuration:**
```
magiklead.com       A     → Vercel IP (or CNAME to cname.vercel-dns.com)
api.magiklead.com   A     → Hetzner VPS IP
```

**Vercel Configuration:**
- Connect GitHub repo, set root directory to `frontend/`
- Set environment variables in Vercel dashboard
- Custom domain: magiklead.com

**Neon Setup:**
- Create database `magiklead`
- Get connection string
- Run migrations

**Post-deploy verification:**
```bash
# Health check
curl https://api.magiklead.com/health

# Test frontend
curl -I https://magiklead.com

# Verify SSL
echo | openssl s_client -servername api.magiklead.com -connect api.magiklead.com:443 2>/dev/null | openssl x509 -noout -dates
```

#### Acceptance Criteria

- [ ] `https://magiklead.com` loads the landing page
- [ ] `https://api.magiklead.com/health` returns 200 "ok"
- [ ] SSL certificates are valid on both domains
- [ ] Clerk sign-up works in production
- [ ] Database migrations have been applied to Neon
- [ ] Stripe webhooks are configured for the production endpoint
- [ ] Clerk webhooks are configured for the production endpoint
- [ ] Docker containers restart on failure (unless-stopped)

#### Notes

- Caddy handles automatic HTTPS — just point DNS and start the container
- Set up a simple deploy script: `ssh root@hetzner "cd magiklead && git pull && docker compose -f docker-compose.prod.yml up -d --build"`
- Monitor with `docker compose logs -f` initially
- Neon free tier includes 0.5 GB storage and 100 hours of compute — sufficient for launch

---

### T20: Polish + QA + Launch Prep

**Day:** 7
**Estimated time:** 3 hours
**Depends on:** T19
**Skill/Agent:** Manual / Claude Code

#### Context

Final pass: fix bugs found during testing, ensure mobile responsiveness, add error boundaries, and prepare Product Hunt launch assets. This is the last task before going live.

#### Inputs

- T19 production deployment working
- All previous frontend and backend tasks

#### Deliverables

- Bug fixes from manual testing
- `frontend/src/app/error.tsx` — global error boundary
- `frontend/src/app/not-found.tsx` — custom 404 page
- `frontend/public/og-image.png` — Open Graph image for social sharing
- Product Hunt draft listing text
- Social media launch post drafts

#### Specification

**QA Checklist:**

1. **Full user flow test:**
   - Visit landing page → click "Start Free" → sign up with Clerk
   - Complete onboarding: paste URL → review profile → review plays → confirm
   - View dashboard with real data
   - View campaign with leads
   - Connect Gmail account
   - Start campaign → verify email is sent to a test address
   - View leads page

2. **Mobile test** (375px, 768px):
   - Landing page sections readable
   - Pricing cards stack properly
   - App sidebar collapses to hamburger
   - Tables scroll horizontally
   - Forms are usable

3. **Edge cases:**
   - Invalid URL in onboarding → error message
   - No Gmail connected → prompt to connect before starting campaign
   - Empty states (no campaigns, no leads)
   - Slow network → loading states visible
   - Token expired → redirect to sign-in

4. **Performance:**
   - Landing page Lighthouse score > 85 (performance)
   - App pages load in < 3s on broadband

**Error boundary** (`src/app/error.tsx`):
```typescript
"use client";
export default function Error({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center">
        <h2 className="text-2xl font-bold">Something went wrong</h2>
        <p className="mt-2 text-muted-foreground">{error.message}</p>
        <button onClick={reset} className="mt-4 rounded bg-primary px-4 py-2 text-white">
          Try again
        </button>
      </div>
    </div>
  );
}
```

**Product Hunt listing draft:**
- **Tagline:** "AI outreach that doesn't cost a fortune"
- **Description:** "MagikLead finds leads matching your ideal customer profile, generates personalized email sequences, and sends them from your Gmail. Starting at $49/mo — 50% cheaper than MoneyPrinter."
- **Topics:** Sales, Email Marketing, AI, Lead Generation, SaaS
- **Maker comment:** Brief backstory about building the alternative

**Social launch posts:**
- Twitter/X: "We just launched MagikLead — AI-powered cold outreach at half the price of [competitors]. Paste your website, get leads, send personalized emails. Starting free. [link]"
- Hacker News: "Show HN: MagikLead — Open-source AI outreach platform (MoneyPrinter alternative)"

#### Acceptance Criteria

- [ ] Full user flow works end-to-end in production
- [ ] No JavaScript console errors on any page
- [ ] Mobile responsive on all pages (375px minimum)
- [ ] Error boundary catches and displays errors gracefully
- [ ] Custom 404 page for missing routes
- [ ] OG image displays when sharing on social media
- [ ] Product Hunt listing draft ready to submit
- [ ] All Lighthouse scores > 80

#### Notes

- Don't spend more than 30 min on any single bug — log it and move on
- The OG image should be 1200x630px — create a simple branded card with the tagline
- Product Hunt launches are best on Tuesday or Wednesday at 12:01 AM PST
- Have a "maker comment" ready explaining why you built this

---

## Dependency Graph

```
T01 (Scaffolding) ──→ T02 (DB Schema) ──→ T09 (Go Server)
                  ──→ T03 (Infra)     ──→ T09
                  ──→ T04 (Landing)   ──→ T05 (Pricing)
                                      ──→ T06 (App Layout) ──→ T07 (Onboarding UI)
                                      ──→ T06              ──→ T08 (Dashboard UI)
                                      ──→ T17 (SEO Pages)
                                      ──→ T18 (Legal + Tech SEO)

T09 (Go Server) ──→ T10 (AI Integration)
                ──→ T11 (Lead Discovery)
                ──→ T12 (Campaign Engine)  ← T10, T11
                ──→ T13 (Gmail OAuth)
                ──→ T15 (Stripe)
                ──→ T16 (Clerk Webhooks)

T07, T08, T10-T13 ──→ T14 (Frontend-Backend Integration)

T14, T15, T16 ──→ T19 (Deploy)
T17, T18      ──→ T19

T19 ──→ T20 (Polish + Launch)
```

**Critical Path:** T01 → T02 → T09 → T10 → T12 → T14 → T19 → T20

---

## Execution Checklist

### Pre-Build (Handled As Tasks Are Reached)

| Item | Needed by | Blocks |
|------|-----------|--------|
| GitHub repo created | T01 (Scaffolding) | Everything |
| Go 1.22+ installed | T01 | Backend tasks |
| Node 20+ / pnpm installed | T01 | Frontend tasks |
| Docker Desktop running | T01 | Local Postgres/Redis |
| Neon database created | T19 (Deploy) | Only production |
| Clerk app created | T06 (App Layout) | Auth tasks |
| Clerk webhook configured | T16 | User sync |
| Stripe account created | T15 (Stripe) | Payments |
| Stripe products/prices created | T15 | Checkout |
| Stripe webhook configured | T15 | Subscription lifecycle |
| Google Cloud project + OAuth credentials | T13 (Gmail) | Gmail integration |
| Anthropic API key | T10 (AI) | AI tasks |
| RapidAPI subscription + key | T11 (Leads) | Lead discovery |
| Domain registered (magiklead.com) | T19 (Deploy) | Production only |
| Hetzner VPS provisioned | T19 (Deploy) | Production only |
| Vercel project linked | T19 (Deploy) | Frontend deploy |

### Post-Build (Day 7)

- [ ] Production deploy verified (both frontend and backend)
- [ ] Stripe test payment works end-to-end
- [ ] Full user flow works: signup → onboarding → campaign → email sent
- [ ] All pages load < 3s
- [ ] Mobile responsive on all pages
- [ ] SEO meta tags on all public pages
- [ ] Legal pages published (Privacy, Terms, Refund)
- [ ] Sitemap.xml and robots.txt accessible
- [ ] Error monitoring active (check Docker logs)
- [ ] DNS propagated for magiklead.com and api.magiklead.com
- [ ] SSL certificates active on both domains
- [ ] Clerk webhooks verified in production
- [ ] Stripe webhooks verified in production
- [ ] Google OAuth callback works in production

---

## Deferred Features (Phase 2 — Week 2+)

| Feature | Effort | Priority |
|---------|--------|----------|
| Chrome Extension (LinkedIn automation) | 3-4 days | P1 |
| Email open/click tracking (pixel + redirect) | 1 day | P1 |
| Reply detection (Gmail polling) | 1 day | P1 |
| Conversations view | 1 day | P2 |
| Sequence editor UI | 1 day | P2 |
| Lead CSV upload | 0.5 day | P2 |
| Campaign analytics charts | 1 day | P2 |
| Multi-tenant switching UI | 0.5 day | P3 |
| A/B testing for email subjects | 2 days | P3 |
| AI reply classification | 1 day | P3 |
| Multiple Gmail accounts per tenant | 0.5 day | P3 |
