# 26 — 7-day public real launch

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#25 — Production quality gate](./25-production-quality-gate.md)

## What to build

Phase 8 of the PRD — the apex gate. Two concurrent real outreach
campaigns on production `magiklead.com`: magikshot and magiktext.
At least 100 prospects each. Real PDL-sourced leads, real Gmail
sends from the operator's connected mailboxes, real replies, real
bounces, real recipients. Public sign-ups are live; strangers can
sign up and join during the seven days.

The hard binary criterion is **zero emails sent to leads who have
replied**. One false positive resets the seven-day clock. The
window is the gate that says "ready" — not the operator's gut.

After this slice, magiklead is a launched product with observable
validation evidence. Any clean seven-day window flips the project
to "live."

## Acceptance criteria

- [ ] Two campaigns created on production:
  - magikshot ICP, ≥100 prospects from PDL, multi-step sequence,
  - magiktext ICP, ≥100 prospects from PDL, multi-step sequence.
- [ ] Both campaigns running concurrently for seven consecutive
      days. Worker sender + poller running the whole time.
      Operator checks at least 2× per day for issues.
- [ ] Outcomes recorded in this file's "Outcome" section, daily:
  - per-campaign send progress (sends attempted, successful,
    failure rate),
  - replies detected, sequence halted within 2 minutes each time,
  - bounces detected, classified correctly (at least one bounce
    in total across both campaigns — PRD requirement),
  - unsubscribes (if any), processed correctly,
  - Sentry P1 count for the day (target: 0),
  - settings page screenshot for each tenant in use (correct
    plan, usage, mailbox status),
  - **zero** emails sent to replied leads (binary, hard-blocking).
- [ ] Public sign-ups working: at least one stranger sign-up
      occurs during the window (verified via Clerk dashboard or
      production logs); their flow completes without 5xx.
- [ ] Any P1 bug pauses the seven-day clock, gets a fix commit,
      and resets the seven-day window. Each pause logged with
      time-stamp + description.
- [ ] PRD severity-policy P1 explicitly recorded as the trigger
      list:
  - any email sent to a replied lead,
  - send failure rate >10%,
  - settings page showing wrong data,
  - sequence not pausing on reply,
  - bounce or unsubscribe silently dropped,
  - privacy erasure broken,
  - site unreachable,
  - Sentry-captured panic in any worker tick.
- [ ] On Day 7 with all the above clean: operator records final
      sign-off — "7-day clean window complete on YYYY-MM-DD;
      magiklead is live."

## Modules touched

- Verification + endurance run, not new code.
- Bug fixes during the window touch whichever module is broken;
  small commits, deployed via the existing deploy script.

## Test prior art

- [#15 — Local 3-day real test](./15-local-3-day-real-test.md) is
  the predecessor at smaller scope.

## Out of scope

- Marketing campaigns / customer acquisition for magiklead itself
  — out of scope per PRD's "What the operator does NOT do during
  this PRD." Strangers sign up via organic discovery or operator's
  personal channels; no paid acquisition.
- Adding new features mid-window — strict freeze except for P1
  fixes.
- Onboarding a paid customer with custom support — if it happens
  organically, the operator handles it ad-hoc; v1 has no support
  process beyond email replies.
- Scaling decisions (more VPS, sharding) — single-VPS for the
  whole window; if load is a problem, that's a Day-8 conversation.
