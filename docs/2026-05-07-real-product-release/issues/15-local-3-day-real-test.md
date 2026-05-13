# 15 — Local 3-day real test

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#14 — Playwright E2E suite](./14-playwright-e2e-suite.md)

## What to build

Phase 5 of the PRD — a three-day endurance run on the local
devpod, real Gmail send, real PDL lookups, real prospects, real
replies, real bounces. The only thing not "real production" is the
URL: still `mvp.magiklead.localhost`. The campaign is for
magikshot (the operator's own product), ~100 prospects sourced via
PDL.

Any P1 bug pauses the three-day clock and restarts the window.
"P1" per PRD: any email sent to a replied lead, send failure rate
above 10%, settings page showing wrong data, sequence not pausing
on reply, bounce or unsubscribe silently dropped, privacy erasure
broken, site unreachable, worker-tick Sentry panic.

After this slice, the operator has lived in their own product for
72 hours under real conditions and has either a clean window or a
list of P1s that get fixed and re-clocked.

## Acceptance criteria

- [ ] Campaign created: real magikshot ICP, ~100 prospects from
      PDL, multi-step sequence (3–5 steps with realistic delays).
- [ ] Campaign run lasts 72 consecutive hours from start. Worker
      sender + poller running the whole time. Operator checks at
      least 3× per day for issues.
- [ ] Outcomes recorded in this file's "Outcome" section:
  - prospects emailed (target: ~100),
  - sends attempted vs. successful (failure rate < 10%),
  - replies detected, sequence halted within 2 minutes each time,
  - bounces detected, classified correctly,
  - unsubscribes (if any), processed correctly,
  - **zero** sends to replied leads (binary, hard-blocking).
- [ ] Any P1 bug pauses the clock, gets a fix commit, and resets
      the 72-hour window. Each pause is logged in this file with
      time-stamp + description.
- [ ] No Sentry panics during the window (note: Sentry isn't wired
      yet locally; for this phase, operator monitors api + worker
      logs directly for panics).
- [ ] Operator records final sign-off: "3-day clean window
      complete on YYYY-MM-DD, ready for production wiring."

## Modules touched

- None (verification + endurance run, not new code).
- Bug fixes during the run touch whichever module is broken;
  small commits onto the magiklead-mvp branch.

## Test prior art

- This is a new endurance-style test. The Playwright suite
  ([#14](./14-playwright-e2e-suite.md)) covers feature regression;
  this slice covers real-conditions regression that automation
  cannot see.

## Out of scope

- Production deploy — that's Phase 6 ([#16](./16-hetzner-vps-and-api.md)
  onwards).
- Public sign-ups — out of scope until Phase 8
  ([#26](./26-seven-day-public-launch.md)). This phase has no
  external users.
- magiktext campaign — runs concurrently in Phase 8 only;
  Phase 5 is magikshot-only to keep the operator's load
  manageable.
- A formal P0/P1/P2 triage process beyond what the PRD specifies.
