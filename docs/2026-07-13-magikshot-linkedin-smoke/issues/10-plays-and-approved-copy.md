# 10 — Plays from magikshot.com analysis (onboarding UI)

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Smoke devpod standup](./01-smoke-devpod-standup.md)

> **Status: done (2026-08-04).** Website analysis + play generation ran
> against magikshot.com for the founder tenant; the 5 plays are live in
> the **onboarding UI**. **Copy authoring + approval moved to
> [#11](./11-campaign-creation-manual-start.md)** and now happen in the
> `/campaigns/new-linkedin` editor — per the founder's 2026-08-05
> direction to run the smoke in the app rather than in `copy.md`/CSV
> files. The former `copy.md` draft was removed.

## What was built

The campaign's raw material — plays anchored on magikshot's real
positioning — produced through the product's own onboarding flow, so the
founder starts #11 from real plays, not a hypothetical (PRD user story
21).

Run 2026-08-04 in the smoke pod as the founder tenant (`be594ede-…`,
Clerk `user_3DIGNMG…`) via the onboarding endpoints:
`POST /api/v1/websites/analyze {"url":"https://magikshot.com"}` → 200
(profile persisted to `tenants.business_profile`), then
`POST /api/v1/plays/generate` → 200 (5 plays persisted, all `active`).
The plays render in the app at **`/onboarding`**; the DB rows are
captured by the nightly dump. Target play for the LinkedIn rail:
**#1 — "LinkedIn Content Creators — Professional Headshots at Scale"**
(ICP: content creators, personal-brand builders, coaches, consultants —
magikshot's own LinkedIn-headshot value prop).

## Acceptance criteria

- [x] Website analysis of magikshot.com completed in the smoke pod;
      generated plays live in the onboarding UI on the founder tenant
      (captured by the nightly DB dump).
- [x] No campaign created — this slice ends before any campaign
      (verified: no LinkedIn campaign rows in the smoke pod).

Copy authoring + founder approval are no longer part of this slice —
they moved to #11 (written and reviewed in the campaign editor). The
send-path constraints that governed the note now live in #11's
*What to build*.

## Modules touched

None (execution slice) — exercised the existing `/websites/analyze` +
`/plays/generate`. No file deliverable: the plays live in the product,
and copy moved to the #11 UI flow.

## Out of scope

- Campaign creation, copy authoring, lead attachment —
  [#11](./11-campaign-creation-manual-start.md).
- Any send.
