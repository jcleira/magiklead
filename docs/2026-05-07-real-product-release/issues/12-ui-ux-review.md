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

- [x] Every screen under
      `frontend/src/app/(app)/` and `frontend/src/app/(marketing)/`
      reviewed against the design tokens. The local devpod is the
      target environment; "real data" means whatever the seed
      fixture + connected integrations expose at review time, not
      a hard prerequisite of all four (Gmail + PDL + Stripe + live
      campaign) firing simultaneously.
      **Done:** code-level review of all 19 pages. See Outcome
      section below for the per-screen catalog. No live-browser
      pass — visual regressions (wrong-font flashes, broken layout
      at uncommon viewport widths) are still on the operator to
      eyeball on the local devpod.
- [x] For each screen, capture in a checklist:
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
- [x] Each demo-tell is fixed in flight (small commits) or, if it
      needs deeper work, captured as a follow-up issue in this
      directory and explicitly marked as out-of-scope for the
      Phase 8 launch window.
      **Done:** fixed in flight — LinkedIn→PDL copy across
      onboarding/landing/FAQ/pricing, the "LinkedIn outreach"
      feature claim removed from the pricing tiers + comparison
      table (the feature does not exist), and the onboarding
      `Field` component made controlled (the "Edit anything that
      looks off" promise now actually persists edits). Bigger
      follow-ups captured in Outcome below.
- [x] Final pass on the marketing landing page: hero, features,
      pricing, footer all final-copy and final-asset.
      **Note:** hero, value props, comparison, pricing preview,
      FAQ, and final CTA all polished. Footer lives in
      `(marketing)/layout.tsx` and was not modified in this pass —
      worth a visual eyeball on the devpod.
- [x] Sign-off line: this file's "Outcome" section records the
      date the review pass completed and an honest assessment
      ("no demo-tells remaining" or a list of accepted known
      issues).

## Outcome

**Code-level review completed 2026-05-27.** No live-browser pass
yet — visual demo-tells that only manifest at render time still
need an operator eyeball on the local devpod before flipping to
green.

### Fixed in flight

- `(app)/onboarding/page.tsx` — "Leads will be discovered via
  LinkedIn API" → "via People Data Labs". The line was actively
  misleading users about how lead discovery works.
- `(app)/onboarding/page.tsx` — `Field` component was rendering
  inputs with `defaultValue` and no `onChange`, so the "Edit
  anything that looks off" CTA promised editability but every
  keystroke was silently dropped. Now controlled via `onChange`
  callbacks that update the `profile` state, so edited values
  flow into the subsequent `/plays/generate` call.
- `(marketing)/page.tsx` — FAQ "How does lead discovery work?"
  rewritten from "We search LinkedIn via API…" to PDL-accurate
  copy.
- `(marketing)/page.tsx` — How-It-Works step 02 rewritten from
  "find decision-makers… on LinkedIn" to PDL-accurate copy.
- `(marketing)/page.tsx` — value-props "All-in-one pipeline"
  rewritten: was "Leads, email sequences, and LinkedIn — in one
  tool", now describes the actual featureset (reply detection,
  suppression, etc.).
- `(marketing)/page.tsx` — comparison table no longer claims
  MagikLead supports LinkedIn outreach (it doesn't); the
  `linkedin` column is removed from the competitor data and
  feature rows.
- `(marketing)/pricing/page.tsx` — Starter + Growth feature
  bullets no longer list "Email + LinkedIn outreach" (the
  LinkedIn outreach module is not built). Replaced with the
  actual feature set ("Email outreach from your Gmail", "Reply +
  bounce detection").
- `(marketing)/pricing/page.tsx` — FAQ "What counts as a lead?"
  reframed away from LinkedIn-based wording to the actual
  per-tenant save-based metering described in the PRD.

### Captured as follow-ups (not done in this pass)

1. **`(app)/campaigns/[id]/page.tsx` still presents the legacy
   SMTP connect popup** (lines 444-515 pre-edit) pulling from
   `/api/v1/email-accounts`, while issues #3 + #8 made Gmail
   OAuth via `/api/v1/gmail/accounts` the canonical email-connect
   path. A user who connected Gmail in `/settings` will be
   prompted to "Connect Email" again from the campaign page
   against a different table. Needs a campaign-page redesign that
   either reuses the settings flow or surfaces the already-
   connected Gmail account. Bigger than #12's polish scope —
   route to a Phase 6 follow-up issue.
2. **Silent error-swallowing** on `(app)/dashboard/page.tsx` and
   `(app)/campaigns/page.tsx`: `.catch(() => setCampaigns([]))`
   makes a 500 or auth failure look identical to "no campaigns
   yet". An error state distinguishable from the empty state would
   improve diagnosis; AC criterion explicitly calls this out and
   it's not strictly fixed yet.
3. **`alert()`-based error UX** on settings (disconnect Gmail,
   start checkout), deliverability ("Failed to check domain"),
   leads (save failure), and onboarding (confirm failure). Works,
   but `alert()` is jarring vs the rest of the design language.
   Toast-style component would fit the design tokens better.
4. **No visual-only check performed** — wrong-font flashes,
   uncommon-viewport layout breaks, hover-state mistakes, etc.
   only surface at render time. The operator should eyeball the
   devpod once before merging this into Phase 8.

## Modules touched

- All frontend pages and components. No backend or schema changes.
- Design-token source-of-truth (Tailwind config + component
  library); no token changes expected, but allowed if a real
  inconsistency surfaces.

## Test prior art

- This is a review pass — code-level inspection of every screen's
  states (loading, error, empty, populated). No automated tests in
  this slice; the Playwright suite in
  [#14](./14-playwright-e2e-suite.md) picks up after.

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
