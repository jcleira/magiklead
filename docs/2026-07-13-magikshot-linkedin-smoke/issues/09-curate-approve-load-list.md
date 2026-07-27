# 09 — Curate, founder-approve, and load the 30–50 prospect list

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Smoke devpod standup](./01-smoke-devpod-standup.md),
[08 — Curated prospect CSV loader](./08-curated-prospect-csv-loader.md)

Approved list: ______ (path + founder sign-off date — MUST be filled
before the load step runs; see Handoff)

## What to build

The smoke's real target list, loaded into the smoke pod (PRD user
stories 16, 17; loading mechanics from 18, 19):

1. **Curate.** 30–50 **face-is-brand professionals** — realtors,
   recruiters, coaches: people whose LinkedIn profile photo *is*
   their storefront, so magikshot's pitch is native and reply
   signal arrives fast. Fill #08's CSV template (name, first/last,
   title, company, company domain, LinkedIn profile URL, location).
   The session may assist with assembly/formatting, but selection
   is founder judgment. Store the CSV **outside the repo** (real
   people's data; house pattern for non-committed artefacts:
   `~/.config/devpods/magiklead/` — record the exact path here).
2. **Approve.** The founder reviews every row before anything is
   loaded — no real person is contacted, or even planted, without
   sign-off. This is the first of the three founder gates.
3. **Load.** Run #08's loader against the **smoke pod**, capture
   the planted/updated/skipped report as evidence. Re-run once to
   demonstrate idempotency on the real list.
4. **Verify searchable** (pass criterion 2): the curated people
   appear in the smoke pod's LinkedIn lead search with their
   profile URLs attached — verified in the UI and/or via the API,
   evidence captured.

## Acceptance criteria

- [ ] CSV of 30–50 rows exists at the recorded out-of-repo path,
      validated by the loader's checks (zero rejected rows in the
      final version, or rejections consciously accepted).
- [ ] `Approved list:` field above is filled BEFORE the first load
      into the smoke pod (US17 — approval precedes loading).
- [ ] Loader report on the smoke pod: all rows planted, counts
      match the CSV; report captured in the docs folder as
      evidence.
- [ ] Second run reports updated/skipped only — no duplicates
      (idempotency against production-shaped data).
- [ ] Lead search in the smoke pod returns the curated prospects
      with LinkedIn URLs attached (criterion-2 evidence: UI
      screenshot or API response capture).
- [ ] Nothing outside the input was touched (loader report is the
      audit trail).

## Modules touched

None (execution slice) — uses #08's CLI in the smoke pod. Evidence
into `docs/2026-07-13-magikshot-linkedin-smoke/`; the CSV itself
stays out of the repo.

## Test prior art

#08's integration tests already prove the mechanics; this slice's
verification is the runbook-style evidence capture (report output +
search results).

## Out of scope

- Campaign creation or any contact with the listed people — #11
  creates the campaign; nothing sends before its explicit start.
- Sourcing prospects via PDL or live LinkedIn search (PRD: curated
  list only).

## Handoff

When the curated CSV is assembled (BEFORE the load step, and again
BEFORE flipping this row in `issues.md`), post this block to the
user:

- **URL / artefact to visit**: the curated CSV at its out-of-repo
  path.
- **Action required**: founder reviews every row and approves the
  list for loading (edit/remove rows as needed).
- **Where to record the decision**:
  - In `issues.md` (one-line note next to this row), AND
  - In this file's `Approved list:` field above (path + date).

The load step MUST NOT run until that field is filled.
