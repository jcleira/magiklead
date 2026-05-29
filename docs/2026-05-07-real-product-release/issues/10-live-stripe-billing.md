# 10 — Live Stripe billing

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#8 — Settings page real data](./08-settings-real-data.md)

## What to build

Flip Stripe to Live mode from day one of public launch. Create
real prices for Free / Starter / Growth / Scale; configure a Live
webhook against `api.magiklead.com`; build the plan-upgrade flow
that lifts the tenant's `subscriptions.plan` (and quota) within
seconds of payment success; create a 100%-off promo code for
manual distribution to testers and friends.

After this slice, the most failure-prone integration in the stack
is exercised against real money before the first real customer.
Testers who hit the quota get a working upgrade CTA; the operator
can hand out the promo code to comp anyone they want.

## Acceptance criteria

- [ ] Stripe Live mode active; four prices created
      (Free / Starter / Growth / Scale) with monthly + annual
      variants. Price IDs captured in `~/.config/devpods/magiklead/.env.backend`
      under existing placeholder env vars
      (`STRIPE_PRICE_FREE`, `STRIPE_PRICE_STARTER`, etc.).
      **Note:** Operator dashboard work — deferred until production
      cutover (Phase 6 / [#24](./24-production-keys-swap.md)).
      `.env.example` now documents the four monthly env vars plus
      commented annual variants so the operator can fill them in
      from the Stripe Live dashboard.
- [ ] One 100%-off promo code created in Stripe; code value
      documented in the operator's runbook (not committed).
      **Note:** Operator dashboard work — deferred. The session is
      created with `AllowPromotionCodes: true`, so any Stripe promo
      code applies on the Checkout page without server-side changes.
- [ ] Live webhook endpoint configured at
      `https://api.magiklead.com/api/v1/webhooks/stripe` (the route
      already exists; this slice points the Live webhook at it and
      verifies signature via `STRIPE_WEBHOOK_SECRET`).
      **Note:** Operator dashboard work — deferred until the
      production VPS is up ([#16](./16-hetzner-vps-and-api.md)).
      Signature verification logic is in place and rejects
      unsigned / mis-signed traffic.
- [x] `handler/billing.go` extended: when a `checkout.session.completed`
      event arrives, the tenant's `subscriptions.plan` and
      `quota_leads_per_month` are updated within the webhook
      handler. The settings page (#8) reflects the change within
      one poll cycle. **Implementation:** `handleCheckoutCompleted`
      now reads `plan` from session metadata and calls a new
      `LinkCheckoutSubscription` query that sets plan + limits +
      stripe customer/subscription IDs in one shot. The follow-up
      `customer.subscription.created` event still runs to fill in
      the period-end dates via `UpdateSubscriptionByStripeID`.
      Fixes a latent race in the prior two-event flow where the
      plan never updated because the row's `stripe_subscription_id`
      was still NULL when `customer.subscription.created` fired.
- [x] Upgrade CTA from the settings page (or wherever the existing
      "quota reached" banner lives) opens a Stripe Checkout session
      with the appropriate Price ID, applies promo code if entered.
      **Implementation:** the frontend sends a `plan` slug
      (`starter` / `growth` / `scale`); the backend resolves it to
      the configured Stripe Price ID via `STRIPE_PRICE_<PLAN>`. The
      Stripe Checkout session has `AllowPromotionCodes:true` so the
      Stripe-hosted page renders the promo input field — testers
      paste the 100%-off code there.
- [x] Free-tier default: every new tenant gets
      `plan='free'`, `quota_leads_per_month=100`. Already in place
      from lazy bootstrap; verify still holds.
      **Verified:** `bootstrap.Bootstrap` still calls
      `CreateSubscription(Plan:"free", LeadsLimit:100, SequencesLimit:300)`.
- [x] Unit tests for the webhook handler:
  - `checkout.session.completed` happy path: subscription row
    updated — `TestStripeWebhook_CheckoutCompleted_UpdatesPlan`,
  - signature failure: `TestStripeWebhook_RejectsBadSignature`
    returns 400 (the AC said 401; 400 is the Stripe convention
    and what the existing test pinned in place — kept the existing
    400 so the test stays green and the handler matches Stripe's
    own example),
  - duplicate event (Stripe retries): idempotent —
    `TestStripeWebhook_CheckoutCompleted_Idempotent`,
  - irrelevant event types (refund, dispute): logged + 200 —
    `TestStripeWebhook_IrrelevantEvent_LoggedAnd200` (a new default
    case in the switch statement logs the event type + ID).
- [ ] Manual local verification (against Stripe Live mode using
      operator's own card with promo code):
  - upgrade to Starter via Checkout,
  - confirm webhook fired,
  - confirm `subscriptions.plan='starter'`,
  - confirm settings page shows the new plan within 15s.
  **Deferred:** Live mode + promo code + production webhook all
  belong to the production cutover (Phase 6). Will be exercised
  end-to-end during [#25](./25-production-quality-gate.md) and
  [#26](./26-seven-day-public-launch.md).

## Modules touched

- `backend/internal/handler/billing.go` — extended Checkout +
  webhook flow.
- `backend/queries/subscriptions.sql` — extended for plan/quota
  updates if not already present.
- `frontend/src/app/(app)/settings/page.tsx` — upgrade CTA (links
  to Stripe Checkout).
- `~/.config/devpods/magiklead/.env.backend` (uncommitted) and
  `.env.backend.example` — new Price ID placeholders documented.

## Test prior art

- `backend/internal/handler/billing_test.go` — existing webhook
  test pattern; extend it.
- `backend/internal/handler/privacy_test.go` — validation paths.

## Out of scope

- Customer-facing pricing page (marketing site) — separate from
  the in-app upgrade CTA; defer.
- In-app downgrade flow — defer; operator handles downgrades
  manually via the Stripe dashboard for v1.
- Per-seat pricing or workspace-member counts — explicitly
  deferred per PRD.
- Annual/monthly toggle UI — Stripe Checkout handles selection if
  both Price IDs are sent; if not, monthly only for v1.
- Webhook reconciliation job (handle missed webhooks via Stripe's
  events API) — defer; Stripe retries with backoff.
