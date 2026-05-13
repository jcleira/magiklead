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

- [ ] New `tests/e2e/` directory at repo root, set up with
      Playwright (Node.js). `package.json` script
      `pnpm e2e` runs the suite locally; `pnpm e2e --ui` opens
      Playwright UI mode.
- [ ] Fifteen test files, one per flow, named to match the flow
      numbers in [#13](./13-d-bar-flow-walkthrough.md):
      `01-signup.spec.ts` ... `15-export-delete.spec.ts`.
- [ ] Each test ends with `await captureArtefact(testInfo)` which
      saves: a full-page screenshot + a JSON snapshot of key DB
      rows (`tenants`, `campaign_leads`, `email_events`,
      `subscriptions`) scoped to the test tenant.
- [ ] Dedicated test Clerk account; dedicated test Gmail Workspace
      with at least 3 mailboxes (one operator-side, two recipient-
      side for reply/bounce). Credentials in
      `~/.config/devpods/magiklead/.env.e2e` (gitignored;
      `.env.e2e.example` documents).
- [ ] Test isolation: each test creates its own tenant (and tears
      it down) so flows don't cross-contaminate. The shared
      external resources (Gmail mailboxes, PDL credits) are reused
      across runs but each run's data is namespaced.
- [ ] GitHub Actions workflow `.github/workflows/e2e.yml` runs
      the suite on every PR + nightly cron. CI fails on any flow
      failing.
- [ ] Each artefact uploaded as a CI artifact; the operator's
      30-second post-CI review is "scroll the artefacts, look for
      surprises."
- [ ] First green run on a branch off main is captured as the
      baseline.
- [ ] One intentional break + revert exercises the failure mode:
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
