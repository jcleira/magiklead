# 11 — LinkedIn campaign: author + create in the app, manual start under the pacer

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [06 — Day-one connect ceremony](./06-day-one-connect-ceremony.md),
[10 — Plays (onboarding UI)](./10-plays-and-approved-copy.md)

**Also blocked (added 2026-09-22)**: the LinkedIn account is
`restricted` (`CREDENTIALS`) since 2026-08-04 — see the status note in
[#06](./06-day-one-connect-ceremony.md). Step 0 clears it.

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

0. **Reconnect the account — Settings (added 2026-09-22).** The bound
   account is `restricted` since 2026-08-04 (status note in
   [#06](./06-day-one-connect-ceremony.md)). It cannot send:
   `pickLinkedInAccount` (`backend/internal/worker/linkedin.go`) takes
   only `active` or `warming` accounts.
   1. **Pod up.** On 2026-09-22 the pod is stopped, since about
      2026-08-17. Run `devpods up` from the smoke worktree — never
      `seed`, never `down` (runbook §0). Then confirm: `devpods status`
      is healthy;
      `curl -si https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile`
      returns 405 (tunnel → api);
      `devpods exec api go run ./cmd/unipile-webhooks list` shows the
      three tunnel webhooks. If they are gone, register them again
      (runbook §1).
   2. **Clean standing.** The founder checks the account's LinkedIn
      notifications and email for a checkpoint or verification near
      2026-08-03/04. If one exists, the runbook §7 *Clean standing* box
      fails: hold, and extend the warm-up as §7 says.
   3. **Reconnect.** The founder clicks **Reconnect** in Settings and
      completes the hosted-auth wizard. This is the first real
      `account.connected` callback through the header-less fix
      (`9db4491`); the 2026-08-03 bind was a replay.
   4. **Verify.** `devpods logs api` shows the callback with a 200, and
      `SELECT id, unipile_account_id, status, last_error FROM linkedin_accounts;`
      returns one row: `v0DboRWMTQiYVSLUNZbmNg`, `active`,
      `last_error` NULL.
   5. **If a second row appears, stop.** The design expects Unipile to
      keep the account id: the upsert is
      `ON CONFLICT (unipile_account_id)`. But the auth-url always mints
      a `type: "create"` link
      (`backend/internal/linkedin/unipile/unipile.go`), and the docs
      record no live reconnect of an existing account. With two rows,
      the worker sends from the new row, but `/api/v1/linkedin/capacity`
      reads the oldest (restricted) row. The old Unipile account also
      breaks the #04 clean slate and the #12 clean disconnect. Record
      it as a defect; the founder decides the cleanup before **Start**.
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

- [ ] Account reconnected (step 0): the api log shows the callback with
      a 200, and `linkedin_accounts` has one row —
      `v0DboRWMTQiYVSLUNZbmNg`, `active`, `last_error` NULL. The
      runbook §7 *Clean standing* box is checked against 2026-08-03/04.
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
