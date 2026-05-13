# 12 — UI/UX design review pass

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#2](./02-suppression-module.md), [#3](./03-gmail-oauth-send.md), [#4](./04-reply-detection.md), [#5](./05-bounce-detection.md), [#6](./06-unsubscribe.md), [#7](./07-pdl-integration.md), [#8](./08-settings-real-data.md), [#9](./09-campaign-metrics.md), [#10](./10-live-stripe-billing.md), [#11](./11-account-export-delete.md)

## What to build

Phase 2 of the PRD — every screen reviewed against the project's
design tokens on the local devpod with real data flowing through
all integrations. The output is a list of demo-tells (skeleton
states, ugly copy, wrong fonts, dead clicks, awkward spacing,
broken empty states, dev-only placeholder strings) and fixes for
each.

This is verification + polish, not new construction. No new
features, no new routes. Anything that requires schema or backend
changes gets pushed to a separate follow-up; this phase only fixes
frontend look-and-feel.

After this phase, the operator and any visitor walking the local
devpod sees a product, not a prototype.

## Acceptance criteria

- [ ] Operator walks every screen under
      `frontend/src/app/(app)/` and `frontend/src/app/(marketing)/`
      with real data (real Gmail account connected, real PDL
      lookups returning, a real campaign mid-progress).
- [ ] For each screen, capture in a checklist:
  - layout matches design tokens (colours, spacing, typography),
  - empty states are intentional (have copy, illustration where
    appropriate, a primary CTA),
  - loading states are intentional (skeleton or spinner — not a
    blank flash),
  - error states are actionable (tell the user what went wrong
    and what to do),
  - copy is professional (no `TODO`, no `lorem`, no developer
    placeholder text),
  - every visible click target actually does something.
- [ ] Each demo-tell is fixed in flight (small commits) or, if it
      needs deeper work, captured as a follow-up issue in this
      directory and explicitly marked as out-of-scope for the
      Phase 8 launch window.
- [ ] Final pass on the marketing landing page: hero, features,
      pricing, footer all final-copy and final-asset.
- [ ] Sign-off line: operator records in this file's "Outcome"
      section the date this phase was completed and an honest
      assessment ("no demo-tells remaining" or a list of accepted
      known issues).

## Modules touched

- All frontend pages and components. No backend or schema changes.
- Design-token source-of-truth (Tailwind config + component
  library); no token changes expected, but allowed if a real
  inconsistency surfaces.

## Test prior art

- This is a HITL review pass. No automated tests in this slice;
  the Playwright suite in [#14](./14-playwright-e2e-suite.md)
  picks up after.

## Out of scope

- Schema or backend behaviour fixes — captured as follow-up
  issues and routed to Phase 1 (re-opened) or a post-launch
  backlog.
- Marketing-site content beyond the landing page (blog, docs) —
  defer.
- Accessibility (WCAG) deep audit — defer; this phase only flags
  obvious a11y demo-tells (missing alt text, contrast violations
  on primary CTAs).
- Mobile-specific polish — magiklead is a desktop product; mobile
  is "looks reasonable on a tablet" not "fully responsive."
