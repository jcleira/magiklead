# 11 — LinkedIn campaign: author + create in the app, manual start under the pacer

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [06 — Day-one connect ceremony](./06-day-one-connect-ceremony.md),
[10 — Plays (onboarding UI)](./10-plays-and-approved-copy.md)

Warm-up start date: 2026-07-29 (account creation; bound 2026-08-03)
Warm-up complete confirmed: ______ (founder, against #05's completion
criterion — the calendar gate)

## What to build

The first real campaign, authored and created **entirely in the smoke
pod UI** the way a customer would, and started only by the founder's
explicit go (PRD user stories 9, 22, 23). This slice now also carries
the **copy gate** that used to be #10: the founder writes and approves
the words in the editor, on screen — no `copy.md`. Campaign mechanics
(pacer, reconcile, withdrawal, reply-halt) are shipped and untouched;
this slice exercises them.

1. **Find prospects — `/leads`.** Search in the app (play #1 ICP:
   content creators / personal-brand builders / coaches / consultants),
   review, and save the target people. No CSV import — the saved search
   *is* the list.
2. **Author + create the campaign — `/campaigns/new-linkedin`.** Pick
   play #1 and write the sequence directly in the editor:
   - **step-0 connection note** — the founder's own words; and
   - **at least one DM** follow-up.

   Attach the saved leads and save as a **draft** (unstarted). The
   founder reviewing the finished draft on screen *is* the copy-approval
   gate (formerly #10's second founder gate) — real people only receive
   words the founder stands behind.

   *Editor constraints, enforced on create* (`validateLinkedinSequence`,
   `backend/internal/handler/campaigns.go`): step-0 note ≤ **300 runes**,
   counted on the raw template including tokens
   (`worker.NoteCharLimit = 300`); the sequence needs **≥ 2 steps**
   (note + ≥1 DM); personalization tokens `{{first_name}}`
   `{{last_name}}` `{{company}}` `{{title}}` are a literal replace with
   **no fallback** (an empty field renders empty), so make sure the
   saved leads carry a first name.
3. **Hold.** Nothing sends on creation: the campaign stays a draft until
   the gates are met — in the app it shows unstarted with zero sent.
4. **Go.** With warm-up complete (calendar gate — 2–3 weeks from the #06
   date, judged against #05's completion criterion), the founder clicks
   **Start** in the campaign UI. First sends flow under the built-in
   warm-up pacer (`backend/internal/linkedin/pacer/pacer.go`): the
   warm-up anchor is stamped on the first invite (`StartLinkedInWarmup`),
   so day one allows at most the week-1 cap (8 invites/day, ramping
   +4/week toward 20/day, weekly cap 100, acceptance-rate breaker at
   <20% once ≥20 invites in the window). The send loop ticks every 60s
   (`StartLinkedInLoop`, `backend/internal/worker/linkedin.go`).

## Acceptance criteria

- [ ] Prospects found + saved via the in-app lead search; they appear
      on the campaign's leads tab (no CSV import).
- [ ] Campaign authored + created in `/campaigns/new-linkedin`: LinkedIn
      channel, step-0 note + ≥1 DM written in the editor, saved as an
      unstarted draft. Founder has reviewed the draft on screen (the
      copy gate).
- [ ] Warm-up-complete gate confirmed by the founder before start.
- [ ] Zero sends before the explicit start — the campaign shows draft /
      0-sent in the app (behind-the-scenes audit: no `linkedin_events`
      rows, worker logs clean for the pre-start window).
- [ ] Founder clicks **Start**; the campaign goes active in the app —
      its start timestamp is the record #12's observation clock runs
      from.
- [ ] Day-1 sends ≤ the week-1 pacer allowance; the rest of the queue
      stays queued (sent-count visible in the app; per-day-vs-cap is the
      behind-the-scenes pacing proof — pass criterion 4 begins here).

## Modules touched

None (execution slice — PRD: campaign mechanics untouched).

## Test prior art

- The e2e campaign spec (`tests/e2e/tests/18-linkedin-campaign.spec.ts`)
  walks this same UI authoring flow against a stub.
- Pacer behavior: unit tests in `backend/internal/linkedin/pacer/`.

## Out of scope

- Daily observation, acceptance/reply handling, withdrawal, and the
  verdict — [#12](./12-run-to-verdict-clean-disconnect.md).
- Any change to pacer constants, send windows, or timezone gating (PRD
  out of scope).

## Handoff

When the draft campaign is created and reviewed (copy gate met) but NOT
started, post this block to the user:

- **Where to look**: the draft campaign in the smoke pod UI
  (`/campaigns` → the new LinkedIn campaign).
- **Action required**: founder confirms warm-up is complete per #05's
  protocol, then clicks **Start** (the go decision).
- **Record**: the campaign's start time in the app is the record;
  #12's observation clock runs from it.
