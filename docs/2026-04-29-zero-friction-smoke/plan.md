## Zero-friction smoke path — Implementation Plan

**Date:** 2026-04-29
**Depends on:** `../2026-04-25-initial-release/plan.md` (T01–T20 + T13 follow-ups + tests landed in the working tree, not yet committed)
**Goal:** Take MagikLead from "sign up requires a terminal command" to "sign up → click → everything works." End state: a fresh user signing up at `http://mvp.magiklead.localhost` can walk the full smoke path (onboarding → search → save → erasure-confirm) without any developer intervention.

---

## Why this exists

The initial-release work shipped behind a Clerk webhook for user/tenant
bootstrap. In a local devpod the webhook isn't reachable from Clerk's
servers (no public tunnel). I worked around that with a dev CLI
(`backend/cmd/dev-create-user`) — but that means every fresh test
needs a terminal. Plus the canonical DB starts empty, so `/leads`
search returns nothing until you remember to run `cmd/seed`.

Both gaps belong in the boot path, not in the human's hands.

---

## Tasks

### T01: Auto-provision tenant on first authenticated request

**Files:**
- `backend/internal/middleware/tenant.go` (rewrite)
- `backend/internal/handler/clerk.go` (extract shared bootstrap fn)
- `backend/cmd/dev-create-user/main.go` (delete — no longer needed)

**What:**
- When `ClerkAuth` produces a valid `clerk_id` + `email` but no
  matching `users` row exists, the next middleware (`WithTenant` or a
  new `EnsureTenant`) creates `users` + `tenants` + `user_tenants` +
  free `subscriptions` rows in one transaction, then continues.
- The webhook handler in `internal/handler/clerk.go` (`user.created`)
  keeps working — production setups still get the prod-grade signed
  webhook path. Both call into a new shared `bootstrapUser(ctx, q,
  clerkID, email, name)` function so the logic isn't duplicated.
- `user.updated` webhook still keeps email/name in sync. The
  middleware path doesn't fire for updates — Clerk's JWT carries the
  current email already.
- Idempotent: a second concurrent first-request handles the unique
  constraint on `users.clerk_id` cleanly (insert-or-find pattern).

**Done when:**
- A fresh Clerk sign-up can hit any protected endpoint and have
  user/tenant rows materialise on the first request, without the
  webhook firing and without `dev-create-user` being run.
- `devpods exec api go run ./cmd/dev-create-user --clerk-id=… …`
  no longer exists in the repo.
- Tests cover the "user already exists" no-op path and the "create on
  first request" happy path.

### T02: Auto-seed canonical DB when empty

**Files:**
- `devpod/compose.yml` (entrypoint shim — extend the existing one)

**What:**
- After migrations apply, the api entrypoint runs a one-line probe
  against `persons`. If `count(*) = 0`, exec `go run ./cmd/seed`
  before launching air.
- Production deploys never hit this branch (the canonical DB has real
  data from Phase 3 ingest), so the check is dev-only by accident
  rather than by config.
- The existing `cmd/seed` already exits cleanly on empty + on already-
  seeded DBs (idempotent via the `(devpod-fixture)` suffix).

**Done when:**
- `devpods down --force && devpods up` produces a freshly migrated DB
  with the 5 orgs + 20 persons fixture loaded automatically.
- `/leads` search for "VP Sales" returns rows immediately on first
  visit; no manual seed step required.

### T03: Walk the smoke path end-to-end

**What:** Verification only — no code changes.

After T01 + T02 land:
1. Visit `http://mvp.magiklead.localhost`, sign up via Clerk's hosted
   page using a real email.
2. Land back at the app, complete onboarding (paste a website URL →
   AI analyzes → pick plays → first campaign created).
3. `/leads` → search "VP Sales" → save 2–3 leads.
4. `/privacy/erasure` (different browser tab) → submit your email →
   open `http://mail-mvp.magiklead.localhost` → click the
   confirmation link.
5. If your email matches `ADMIN_EMAILS`, `/conflicts` and
   `/persons/<id>` should also be reachable.

**Done when:** All five steps succeed without opening a terminal.

---

## What's intentionally NOT in this plan

- **Production Clerk webhook setup.** Still required for prod (it's
  faster than lazy bootstrap and catches `user.updated` events
  cleanly). T01 just stops it being a *prerequisite* for local dev.
- **Stripe webhook tunneling.** That's already handled by the
  `stripe-listen` devpods module — no equivalent change needed.
- **Real ingest data.** Phase 3 of the prior plan (T08–T10) handles
  real ingest; the dev fixture is enough for smoke testing.

---

## Executor notes

This plan is small (≈ 100 lines of code + a one-line shell change) and
self-contained. A fresh session can:

1. Read this file and `../2026-04-25-initial-release/plan.md` for
   context on what shipped.
2. Read `backend/internal/middleware/tenant.go` and
   `backend/internal/handler/clerk.go` to understand the current
   user-bootstrap flow.
3. Execute T01 and T02 in order.
4. Walk T03 to verify.

The working tree currently has uncommitted changes from the prior
plan; do not commit them yet — the new plan should land as additional
work on top, then the whole thing can be reviewed in one diff.
