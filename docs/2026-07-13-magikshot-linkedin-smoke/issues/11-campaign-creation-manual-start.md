# 11 — Campaign creation + explicit manual start under the pacer

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [06 — Day-one connect ceremony](./06-day-one-connect-ceremony.md),
[09 — Curate, approve, load the list](./09-curate-approve-load-list.md),
[10 — Plays + approved copy](./10-plays-and-approved-copy.md)

Warm-up start date: 2026-07-29 (account creation; bound 2026-08-03) — filled from #06's handoff
Warm-up complete confirmed: ______ (founder, against #05's
completion criterion — the calendar gate)
Approved copy: ______ (filled from #10's handoff)

## What to build

The first real campaign, created from approved materials and started
only by the founder's explicit go (PRD user stories 9, 23). Campaign
mechanics are **untouched** — the shipped pacer, reconcile,
withdrawal, and reply-halt run as-is; this slice exercises them, it
does not modify them.

1. **Create** the LinkedIn campaign in the smoke pod UI: sequence =
   step-0 connection note + the DM step(s), **verbatim** from the
   approved `copy.md`; targets = the curated prospects from #09
   (search → select/save). Verify pass criterion 3 by diffing the
   campaign's stored steps against `copy.md` (SQL dump of the
   campaign/steps rows vs. the approved text).
2. **Hold.** Nothing sends on creation: the campaign stays unstarted
   until the gates above are all filled. Verify no
   `linkedin_events` exist for it and worker logs show no send
   attempts.
3. **Go.** With warm-up complete (calendar gate — 2–3 weeks from
   the #06 date, judged against #05's completion criterion), the
   founder explicitly starts the campaign. First sends flow under
   the built-in warm-up pacer
   (`backend/internal/linkedin/pacer/pacer.go`): the warm-up anchor
   is stamped on the first invite (`StartLinkedInWarmup`), so day
   one allows at most the week-1 cap (8 invites/day, ramping +4 per
   week toward 20/day, weekly cap 100, acceptance-rate breaker at
   <20% once ≥20 invites in the window). The send loop ticks every
   60s (`StartLinkedInLoop`, `backend/internal/worker/linkedin.go`).

## Acceptance criteria

- [ ] All three gate fields above are filled before start.
- [ ] Campaign exists in the smoke pod: LinkedIn channel, steps
      byte-equal to the approved copy (diff evidence captured).
- [ ] Attached leads = exactly the approved list's people (count
      matches #09's loaded set; no strays) — SQL evidence.
- [ ] Zero sends before the explicit start (no `linkedin_events`
      rows for the campaign; worker logs clean for the pre-start
      window).
- [ ] Founder's go recorded (date/time) in `issues.md` and in #12's
      `Campaign start date:` field.
- [ ] Day-1 sends ≤ the week-1 pacer allowance; the remainder of
      the queue stays queued (SQL count of day-1 invite events vs.
      the cap — pass criterion 4 starts accruing here).

## Modules touched

None (execution slice — PRD: campaign mechanics untouched).
Evidence into `docs/2026-07-13-magikshot-linkedin-smoke/`.

## Test prior art

- The e2e campaign spec (`tests/e2e/tests/18-linkedin-campaign.spec.ts`)
  walks the same authoring flow against a stub — use its
  assertions as the shape of the evidence queries.
- Pacer behavior: unit tests in `backend/internal/linkedin/pacer/`.

## Out of scope

- Daily observation, acceptance/reply handling, withdrawal, and the
  verdict — [#12](./12-run-to-verdict-clean-disconnect.md).
- Any change to pacer constants, send windows, or timezone gating
  (PRD out of scope).

## Handoff

When the campaign is created and verified (ACs 1–4) but NOT yet
started, post this block to the user:

- **URL / artefact to visit**: the campaign in the smoke pod's UI +
  the steps-vs-copy diff evidence.
- **Action required**: founder confirms warm-up is complete per the
  protocol and explicitly starts the campaign (the go decision).
- **Where to record the decision**:
  - In `issues.md` (start date next to this row), AND
  - In [12 — Run to verdict](./12-run-to-verdict-clean-disconnect.md)
    under its `Campaign start date:` field.

Issue #12's observation clock runs from that date.
