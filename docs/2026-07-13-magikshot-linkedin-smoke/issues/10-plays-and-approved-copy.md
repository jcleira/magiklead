# 10 — Plays from magikshot.com analysis + hand-written, approved copy

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Smoke devpod standup](./01-smoke-devpod-standup.md)
(runs in the smoke pod; can proceed during warm-up, in parallel with
#09)

## What to build

The campaign's words, anchored on the product's real positioning and
approved by the founder before any campaign exists (PRD user stories
21, 22):

1. **Plays from website analysis.** In the smoke pod (requires
   `ANTHROPIC_API_KEY` in `~/.config/devpods/magiklead/.env.backend`),
   run the onboarding flow against **magikshot.com** —
   `/websites/analyze` then `/plays/generate` (handlers in
   `backend/internal/handler/`, generation in
   `backend/internal/ai/`) — so the plays reflect magikshot's
   actual positioning, not a hypothetical. If the founder's tenant
   was already onboarded with magikshot.com at day-0 (#06 prep),
   this step is verification + capture rather than a fresh run.
2. **Hand-written copy.** Starting from the generated plays as
   raw material, the founder hand-writes:
   - the **step-0 connection note** — must fit within the invite
     note character limit the send path enforces (check the
     constraint in the campaign editor / `SendInvitation` path in
     `backend/internal/linkedin/unipile/` during execution and
     state it in the copy file), and
   - **at least one DM follow-up**.
   Draft in `copy.md` in this docs folder — the founder's words are
   fine to commit.
3. **Approve.** Second founder gate: real people only receive words
   the founder stands behind. The approved final text is the
   authoritative source #11 diffs the campaign against.

## Acceptance criteria

- [ ] Website analysis of magikshot.com completed in the smoke pod;
      generated plays visible in-app (evidence captured — screenshot
      or API response).
- [ ] `copy.md` committed: final connection note (with its
      character count and the enforced limit noted) + at least one
      DM, marked as founder-authored.
- [ ] Founder approval recorded: in `issues.md` next to this row
      AND in #11's `Approved copy:` field.
- [ ] No campaign has been created yet — this slice ends at
      approved words (verified: no LinkedIn campaign rows in the
      smoke pod).

## Modules touched

None expected (execution slice) — exercises existing
`/websites/analyze` + `/plays/generate`. Deliverable is
`docs/2026-07-13-magikshot-linkedin-smoke/copy.md`.

## Test prior art

N/A — the flow is covered by existing e2e onboarding specs under
`tests/e2e/tests/`; this slice's output is judged by the founder,
not tests.

## Out of scope

- Campaign creation and lead attachment —
  [#11](./11-campaign-creation-manual-start.md).
- Any send.
- Multi-variant copy testing — one approved note + DM set is the
  smoke's scope.

## Handoff

When the draft copy is ready (and before flipping this row in
`issues.md`), post this block to the user:

- **URL / artefact to visit**:
  `docs/2026-07-13-magikshot-linkedin-smoke/copy.md` (draft) + the
  generated plays in the smoke pod UI.
- **Action required**: founder rewrites/edits and approves the
  final connection note + DM copy.
- **Where to record the decision**:
  - In `issues.md` (one-line note next to this row), AND
  - In [11 — Campaign creation + manual start](./11-campaign-creation-manual-start.md)
    under its `Approved copy:` field (point at the approved
    `copy.md` revision).

Issue #11 MUST NOT start until that field is filled.
