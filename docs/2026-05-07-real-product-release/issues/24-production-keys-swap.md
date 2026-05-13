# 24 — Production Clerk + Stripe keys swap

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#16 — Hetzner VPS + api online](./16-hetzner-vps-and-api.md), [#10 — Live Stripe billing](./10-live-stripe-billing.md)

## What to build

Final production cutover: swap the dev Clerk keys + dev Stripe
keys on the Hetzner deploy for production keys, configure the
production Clerk JWT template + webhook, point the Stripe Live
webhook at `api.magiklead.com`, and enable public sign-ups (no
invite-only gate). After this slice, `magiklead.com` is a fully
live product accepting strangers' sign-ups against production
identity and Live-mode billing.

This is a deliberately isolated slice — production keys are the
single point of "this is now real, the dev tap is closed" — and
it must be the last thing flipped before [#25 — Production quality
gate](./25-production-quality-gate.md). If anything fails after
this slice, the operator can revert to dev keys without touching
the rest of the deploy.

## Acceptance criteria

- [ ] Production Clerk instance created (separate from the dev
      instance). Production publishable + secret keys captured;
      `CLERK_PUBLISHABLE_KEY` (backend), `CLERK_SECRET_KEY`
      (backend), `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` (frontend)
      updated on the VPS production env.
- [ ] Production Clerk JWT template `magiklead-backend` created
      with body `{"email": "{{user.primary_email_address}}"}` —
      same shape as dev. Without it the backend's admin gate
      returns 403.
- [ ] Production Clerk webhook endpoint configured pointing at
      `https://api.magiklead.com/api/v1/webhooks/clerk` with
      events `user.created` + `user.updated`.
      `CLERK_WEBHOOK_SECRET` updated on the VPS env.
- [ ] Stripe Live webhook (configured in [#10](./10-live-stripe-billing.md))
      verified pointing at `https://api.magiklead.com/api/v1/webhooks/stripe`
      with the correct signing secret on the VPS env.
- [ ] Public sign-ups enabled — Clerk's "Restrictions" settings
      do not block any email; the marketing landing page's "Sign
      up" CTA is visible and works for a stranger from a fresh
      browser.
- [ ] api restart on the VPS; first sign-up against production
      Clerk lands a tenant via lazy bootstrap; settings page shows
      the new tenant with `plan='free'`, `quota=100`.
- [ ] Smoke test with the operator's secondary email: sign up,
      see dashboard, no 5xx. Confirm production Clerk dashboard
      shows the new user.
- [ ] Rollback plan documented in `deploy/README.md`: how to
      revert env vars to dev keys + restart, in case the cutover
      fails.

## Modules touched

- Production env file (`/etc/magiklead/.env.production` on the
  VPS) — only env variable values change.
- Clerk dashboard (production instance setup).
- Stripe dashboard (Live webhook endpoint already created in #10;
  verified here).

## Test prior art

- The working-MVP smoke walk
  (`../../2026-04-30-working-mvp/issues/04-local-smoke-walkthrough.md`)
  exercised lazy bootstrap against dev Clerk; this slice exercises
  the same flow against production Clerk.

## Out of scope

- Production Stripe Live setup itself — done in
  [#10](./10-live-stripe-billing.md). This slice only points the
  webhook URL at production.
- Marketing-site copy changes for public launch — defer to a
  separate marketing slice if needed; the Phase 2 UI/UX review
  ([#12](./12-ui-ux-review.md)) covers the landing-page polish.
- Authentication providers beyond email-password + Google OAuth
  (which Clerk handles) — defer.
- Custom Clerk login UI — Clerk's hosted page is the v1 default.
