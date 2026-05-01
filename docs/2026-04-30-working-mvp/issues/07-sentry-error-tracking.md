# 07 — Sentry error tracking

**Type**: HITL — needs Sentry account creation + DSN provisioning.
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#4 — Hetzner VPS + domain + api online](./04-hetzner-vps-and-api.md)

## What to build

Wire Sentry into both backend (Go SDK) and frontend (Next.js SDK). Errors from production — panics, 5xx responses, unhandled promise rejections — are reported with the Clerk user ID and tenant ID attached so they're attributable. After this slice, an intentional test error in production appears in the Sentry dashboard within ~60 seconds.

## Acceptance criteria

- [ ] Sentry account exists; project created (Go + Next.js, separate or shared); DSN provisioned for each.
- [ ] Backend imports `getsentry/sentry-go`, initialises with the DSN at api startup. Missing DSN logs a warning and continues so dev runs without a DSN.
- [ ] Backend has panic-recovery middleware (or extends the existing chi recoverer) that captures panics + request context (Clerk user ID + tenant ID as Sentry user/tag) and forwards to Sentry.
- [ ] Backend reports 5xx responses to Sentry with the same context (an HTTP-status-aware reporter on the response writer).
- [ ] Frontend imports `@sentry/nextjs`, configured per Sentry's Next.js wizard. User context populated from `useAuth` (or whichever Clerk hook gives the Clerk ID).
- [ ] Production env files include `SENTRY_DSN` (backend) and `NEXT_PUBLIC_SENTRY_DSN` (frontend); `.env.production.example` documents both.
- [ ] Trigger an intentional test error in prod (e.g. a temporary `/sentry-test` endpoint or a frontend button); confirm the event appears in Sentry within 60 seconds with the expected user ID and tenant ID. Remove the test trigger after verification.

## Modules touched

- Sentry integration (new — backend SDK + middleware, frontend SDK + provider wrapping).

## Test prior art

- `backend/internal/middleware/logging.go` — pattern for request middleware that extracts context (Clerk ID, etc.) for downstream use.
- `backend/internal/middleware/admin_test.go` — pattern for testing middleware with manipulated context.

## Out of scope

- Source map upload to Sentry — nice-to-have; defer if not trivial in the Next.js wizard flow.
- Custom Sentry alerts (Slack/email rules) — defer post-launch.
- Performance monitoring / tracing — defer; error tracking is the v1 ask.
