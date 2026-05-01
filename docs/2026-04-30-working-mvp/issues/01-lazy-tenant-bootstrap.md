# 01 — Lazy tenant bootstrap

**Type**: AFK
**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A new internal package encapsulates the four-row provisioning sequence (`users`, `tenants`, `user_tenants`, free `subscriptions`) inside a single Postgres transaction. The protected-routes middleware calls this on first request when no user row exists for the Clerk identity. The Clerk webhook handler is refactored to call the same function on `user.created`. The legacy `cmd/dev-create-user` CLI is deleted.

After this slice, a fresh Clerk sign-up reaching any protected endpoint at `mvp.magiklead.localhost` materialises all four rows automatically — no terminal command, no webhook prerequisite for local dev.

## Acceptance criteria

- [ ] New `internal/bootstrap` package exposes one function: takes a `*pgxpool.Pool`, Clerk ID, email, and optional display name; returns a tenant ID and an error.
- [ ] All four inserts run inside one Postgres transaction; any failure rolls back the entire sequence.
- [ ] The user write uses `INSERT ... ON CONFLICT (clerk_id) DO UPDATE` so concurrent first-requests don't double-insert; a `SELECT ... FOR UPDATE` on the user row inside the tx serialises the rest of the bootstrap.
- [ ] The function returns a typed "no email" error when the JWT email claim is empty; the middleware surfaces this as HTTP 503 with a body naming the JWT-template configuration as the cause.
- [ ] Workspace name fallback uses the email's local-part with a possessive suffix (`tim's Workspace`) when no display name is provided.
- [ ] `WithTenant` middleware renamed to `EnsureTenant`; on missing user it calls bootstrap; on bootstrap error it returns 5xx and logs.
- [ ] `internal/handler/clerk.go` `handleUserCreated` refactored to call the bootstrap function. `handleUserUpdated` left untouched.
- [ ] `backend/cmd/dev-create-user` directory deleted; no references remain in the repo (grep returns only the PRD/plan history files).
- [ ] New sqlc query `UpsertUserByClerkID` added to `backend/queries/users.sql`; sqlc-generated Go regenerated and committed.
- [ ] Tests: bootstrap module — happy path, idempotent re-call, transaction rollback (against a real Postgres). Middleware — user exists vs missing, with a stub bootstrap.
- [ ] `go test ./...` passes; `go vet ./...` clean.
- [ ] Manual: `devpods up` on a fresh devpod, sign in via Clerk hosted page, hit any protected endpoint — user/tenant/role/subscription rows appear in Postgres. No terminal command needed.

## Modules touched

- Tenant bootstrap (deep module — new internal package).
- Tenant-resolution middleware (modified).
- Clerk webhook handler (modified).
- User upsert query (new sqlc query).

## Test prior art

- `backend/internal/middleware/admin_test.go` — pattern for middleware tests with manipulated context and stub handlers.
- `backend/internal/handler/*_test.go` — handler tests run against `nil` queries and exercise validation paths only. The bootstrap module is the exception (real Postgres) because the transactional guarantee is the substantive thing under test — see PRD `## Testing Decisions`.

## Out of scope

- Devpods seed snapshot — see [issue #2](./02-devpods-seed-snapshot.md).
- Production deployment — see [issue #4](./04-hetzner-vps-and-api.md), [#5](./05-frontend-on-hetzner.md).
- Webhook `user.updated` flow — left untouched per PRD.
- Pre-emptive validation that the JWT template is configured at startup — explicitly rejected; surface the misconfig at request time instead.
