# 13 — Operator D-bar flow walkthrough

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#12 — UI/UX design review pass](./12-ui-ux-review.md)

## What to build

Phase 3 of the PRD — the operator manually walks every D-bar user
flow on the local devpod with real data and explicitly marks each
one as matching expectation. The fifteen flows are the canonical
list of "the things magiklead does," derived from the user-stories
section of the PRD; the operator's signature on each one is the
gate that says "this flow works."

Bugs found here are fixed in flight (small commits, then re-walk)
or, if deeper, captured as follow-up issues that block this slice
from being marked complete.

## Acceptance criteria

The fifteen D-bar flows from the PRD's user-stories section,
walked end-to-end on the local devpod, each producing a signed-off
line in this file's "Outcome" section. Order matches the user-
stories grouping:

- [ ] **Flow 1**: Sign-up at `mvp.magiklead.localhost` via Clerk →
      land on dashboard with no errors.
- [ ] **Flow 2**: Lazy bootstrap — first protected request
      materialises tenant + free subscription. (Already proved by
      working-MVP #1 + #4; re-verify under real-product state.)
- [ ] **Flow 3**: Settings page shows real workspace name, plan,
      usage, connected mailboxes (per [#8](./08-settings-real-data.md)).
- [ ] **Flow 4**: Connect Gmail via OAuth, see "connected" badge,
      disconnect and reconnect.
- [ ] **Flow 5**: Paste website → AI plays → first campaign
      created.
- [ ] **Flow 6**: Lead search with structured filters returns real
      PDL-backed persons with verified emails (per
      [#7](./07-pdl-integration.md)).
- [ ] **Flow 7**: Save 5-10 leads to a campaign; quota counter
      increments by exactly the number of unique persons.
- [ ] **Flow 8**: Generate sequence from campaign ICP/play; edit
      step 2's subject and body; save.
- [ ] **Flow 9**: Start campaign; step 1 sends from connected
      Gmail to all active leads; `gmail_message_id` recorded.
- [ ] **Flow 10**: Reply from one recipient mailbox; within ~2
      minutes, that lead's `campaign_lead.status='replied'`; no
      further sends fire.
- [ ] **Flow 11**: Bounce simulation (send to a deliberately bad
      address); within ~2 minutes, bounce event row + lead
      `bounced`.
- [ ] **Flow 12**: Click one-click unsubscribe header in Gmail UI;
      that lead skipped on next tick.
- [ ] **Flow 13**: Campaign-metrics dashboard shows accurate
      counts (per [#9](./09-campaign-metrics.md)).
- [ ] **Flow 14**: Upgrade plan via Stripe Live Checkout with the
      100%-off promo code; settings reflects new plan within 15s
      (per [#10](./10-live-stripe-billing.md)).
- [ ] **Flow 15**: Export account data → unzip → spot-check;
      delete account → confirm cascade + Clerk delete (per
      [#11](./11-account-export-delete.md)).

For each flow above, record in this file's "Outcome" section: a
date stamp, "PASS" / "PASS with notes" / "FAIL → fix in flight",
and a one-line comment.

## Modules touched

- None directly (HITL verification pass). Bugs fixed in flight
  touch whichever module is broken.

## Test prior art

- The working-MVP local smoke walk
  (`../../2026-04-30-working-mvp/issues/04-local-smoke-walkthrough.md`)
  is the predecessor. This walk is its real-product successor with
  real Gmail / real PDL / real Stripe Live.

## Out of scope

- Production walk-through — see
  [#25 — Production quality gate](./25-production-quality-gate.md).
- Automated coverage of these flows — see
  [#14 — Playwright E2E suite](./14-playwright-e2e-suite.md).
- 3-day or 7-day endurance runs — see
  [#15](./15-local-3-day-real-test.md) and
  [#26](./26-seven-day-public-launch.md).
