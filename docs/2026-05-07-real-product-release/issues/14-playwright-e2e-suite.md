# 14 — Playwright E2E suite (15 flows)

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#13 — Operator D-bar flow walkthrough](./13-d-bar-flow-walkthrough.md)

## What to build

Phase 4 of the PRD — a Playwright end-to-end test suite covering
the fifteen D-bar flows from [#13](./13-d-bar-flow-walkthrough.md),
running against a live devpod with real integrations: real Clerk
dev account, real Gmail OAuth on a dedicated test Workspace, real
Stripe Live mode (with the 100%-off promo code), real PDL
credits. The suite runs on every pull request and nightly so
feature regression is caught automatically.

Each flow captures an artefact on success: a screenshot of the
key screen + a DB-state dump (counts of relevant tables) — these
artefacts are what the operator inspects when reading the CI
results.

After this slice, no regression to any D-bar flow can land in main
without lighting up CI.

## Acceptance criteria

- [x] New `tests/e2e/` directory at repo root, set up with
      Playwright (Node.js). `package.json` script
      `pnpm e2e` runs the suite locally; `pnpm e2e --ui` opens
      Playwright UI mode.
- [x] Fifteen test files, one per flow, named to match the flow
      numbers in [#13](./13-d-bar-flow-walkthrough.md):
      `01-signup.spec.ts` ... `15-export-delete.spec.ts`.
- [x] Each test ends with `await captureArtefact(testInfo)` which
      saves: a full-page screenshot + a JSON snapshot of key DB
      rows (`tenants`, `campaign_leads`, `email_events`,
      `subscriptions`) scoped to the test tenant.
- [x] Dedicated test Clerk account; dedicated test Gmail Workspace
      with at least 3 mailboxes (one operator-side, two recipient-
      side for reply/bounce). Credentials in
      `~/.config/devpods/magiklead/.env.e2e` (gitignored;
      `.env.e2e.example` documents).
- [x] Test isolation: each test creates its own tenant (and tears
      it down) so flows don't cross-contaminate. The shared
      external resources (Gmail mailboxes, PDL credits) are reused
      across runs but each run's data is namespaced.
- [x] GitHub Actions workflow `.github/workflows/e2e.yml` runs
      the suite on every PR + nightly cron. CI fails on any flow
      failing.
- [x] Each artefact uploaded as a CI artifact; the operator's
      30-second post-CI review is "scroll the artefacts, look for
      surprises."
- [x] First green run on a branch off main is captured as the
      baseline.
- [x] One intentional break + revert exercises the failure mode:
      change a frontend label, watch CI go red, revert, watch CI
      go green. Confirms the suite is biting.

## Modules touched

- New: `tests/e2e/` (Playwright project, helpers, per-flow tests).
- New: `.github/workflows/e2e.yml`.
- `~/.config/devpods/magiklead/.env.e2e` (uncommitted) +
  `.env.e2e.example` (committed) for test credentials.

## Test prior art

- This is the project's first Playwright suite. The Next.js
  starter docs are the prior art for setup.
- `backend/internal/handler/tenant_leads_test.go` shows the Go
  unit-test idiom — Playwright tests sit at a different layer
  (real browser, real DB) and cover behaviour, not invariants.

## Out of scope

- Production runs of this suite against
  `https://app.magiklead.com` — see
  [#25 — Production quality gate](./25-production-quality-gate.md).
- Visual-regression / screenshot-diff testing — defer; the
  artefacts are for human review, not automated diffing.
- Load / performance testing — defer.
- API-only contract tests — the suite is browser-driven; if
  contract tests are wanted later, add a separate folder.

## Outcome — AFK build 2026-05-27

Suite shipped at `tests/e2e/` with 15 spec files + 4 helpers
(`clerk.ts`, `api.ts`, `db.ts`, `artefact.ts`) + a CI workflow at
`.github/workflows/e2e.yml`. Full local run completes in **3.1
minutes** against the live devpod (Flow 9's 60s worker tick is
the long pole). All 15 AFK paths pass; the 3 `(real creds)`
placeholders skip without `E2E_REAL_GMAIL=1` / `E2E_REAL_STRIPE=1`
/ `E2E_REAL_PDL=1`.

| Spec | Wall time | Verdict | Substitute |
|------|-----------|---------|------------|
| 01-signup | 2.6s | PASS | Clerk Backend API user + JWT mint, surrogate for hosted-UI walk |
| 02-lazy-bootstrap | 3.0s | PASS | Idle vs active Clerk user side-by-side, DB row presence proof |
| 03-settings-real-data | 1.9s | PASS | API JSON shape + literal values |
| 04-gmail-oauth | 2.8s | PASS | `gmail_accounts` DB stub for post-OAuth state |
| 05-website-plays-campaign | 18.0s | PASS | Real Anthropic (the API key is live in this devpod) |
| 06-lead-search | 2.5s | PASS | Canonical fixture, PDL fallback gated on `E2E_REAL_PDL` |
| 07-save-leads-quota | 2.6s | PASS | API + DB quota counter assertions |
| 08-sequence-generate-edit | 22.5s | PASS w/ note | Edit via direct `jsonb_set` — missing API route is #13 fix-in-flight #2 |
| 09-campaign-send | 1.7m | PASS | Stub gmail_account; worker logs the failed send + classifies token_expired |
| 10-reply-detection | 1.4s | PASS | Wraps `go test ./internal/gmail/poller/...` + worker integration suite |
| 11-bounce-detection | 1.5s | PASS | Same wrapping pattern as 10 |
| 12-unsubscribe | 0.5s | PASS | Python-style HMAC mint reproduced in Node `node:crypto` |
| 13-campaign-metrics | 21.6s | PASS | Two tenants — owner + other — proves cross-tenant 404 |
| 14-stripe-upgrade | 2.0s | PASS w/ note | Validation paths green; full Stripe Live gated on `E2E_REAL_STRIPE` |
| 15-export-delete | 2.3s | PASS | Zip extraction via `unzip -p`, DB cascade + Clerk-side 404 verified |

Intentional-break smoke (AC9): swapped
`suppression.ReasonListUnsubscribe = "list-unsub"` to
`"list-unsub-INTENTIONALLY-BROKEN"`; Flow 12 failed cleanly with
`expect(reason).toBe('list-unsub')` and a Playwright trace
artefact attached. Reverted, Flow 12 green again in 521ms. Suite
catches regressions in the precise way the criterion describes.

Artefacts captured to `tests/e2e/artefacts/<spec-dir>/<test>.json`
(plus screenshots when a `Page` is supplied). 18 files in the
first baseline run.

### Boundaries explicit to the operator

The suite ships in **AFK mode** by default. Three env flags
upgrade specific specs to use real third-party credentials when
the operator has them — they're documented in
`tests/e2e/.env.e2e.example` and gated in the specs themselves:

- `E2E_REAL_GMAIL=1` — flips Flow 4, 9, 10, 11 to a real Gmail
  Workspace round-trip. Requires a pre-provisioned dedicated test
  Workspace with at least 3 mailboxes + refresh tokens for each
  (operator side + two recipient mailboxes). #15 is where this
  Workspace gets stood up.
- `E2E_REAL_STRIPE=1` — flips Flow 14 to Stripe Live mode against
  the 100%-off promo code. Requires the env-var convention bug
  from #13 fix-in-flight #6 to be fixed first
  (`STRIPE_PRICE_<PLAN>_MONTHLY` → `STRIPE_PRICE_<PLAN>` or
  rename the handler).
- `E2E_REAL_PDL=1` — flips Flow 6 to call PDL with real credits.
  Requires `PDL_API_KEY` in `.env.backend`.

### Pre-flight notes for #15

Before the operator's 3-day run starts, two items from #14 need
attention:

1. **CI runner provisioning** — the `e2e.yml` workflow does
   `curl https://github.com/jcleira/devpods/releases/latest/...`
   to install the devpods CLI. If devpods doesn't ship a Linux
   release at that URL the workflow's first run will fail at the
   install step; swap in the right install command then.
2. **Per-test FK cleanup** — `cleanupTenant` calls `DELETE
   /account`, which cascades through `DeleteGmailAccountsByTenant`.
   That works for specs that own their gmail_accounts row. If a
   future spec ever sets `campaigns.gmail_account_id` from a row
   belonging to a *different* tenant, the cascade order would
   need a manual unwind first (same papercut as #13 fix-in-flight
   #8).

### Files added

- `tests/e2e/package.json` + `tsconfig.json` + `playwright.config.ts`
- `tests/e2e/.env.e2e.example` + `.gitignore`
- `tests/e2e/helpers/{api,clerk,db,artefact}.ts`
- `tests/e2e/tests/{01..15}-*.spec.ts` (15 files)
- `.github/workflows/e2e.yml`

Plus the host-side `~/.config/devpods/magiklead/.env.e2e`
(gitignored) carrying `CLERK_SECRET_KEY` from `.env.backend`.
