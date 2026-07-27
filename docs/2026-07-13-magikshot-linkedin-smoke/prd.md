# magikshot LinkedIn Smoke — real sends from the laptop

## Problem Statement

magiklead's LinkedIn rail is feature-complete on paper — hosted-auth
connect, warm-up pacer, acceptance breaker, reply-halt, reconcile,
withdrawal — but it has never carried a real message for a real
customer. Meanwhile magikshot needs leads. The founder wants to run
magikshot's first real outbound campaign *through* magiklead, so the
product validates itself on its own outreach before any external
customer touches it.

Doing this today is blocked three ways:

1. **No production lead source.** The only thing that ever plants
   LinkedIn-usable identifiers in the canonical person graph is the
   dev seed's synthetic fixtures. The ingest pipeline writes LinkedIn
   identifiers under a divergent type/format the search and send rail
   never read, so in a real environment LinkedIn lead search returns
   zero rows.
2. **Webhooks can't reach the laptop.** The devpod's API host is
   unresolvable from Unipile's cloud, so connected-account binding,
   acceptance events, and inbound replies never arrive locally.
3. **There is no production deployment** — and the founder explicitly
   does not want to stand one up for this. The smoke must run
   entirely from the laptop while remaining a real production thing:
   real LinkedIn account, real Unipile subscription, real invites and
   DMs to real prospects.

## Solution

Run the smoke from a **dedicated smoke devpod** on the laptop,
reachable by Unipile through a **stable named cloudflared tunnel**
(hostname on the magikshot.com Cloudflare zone), with webhooks
registered once against that hostname.

Fix the LinkedIn identifier divergence so every data source feeds the
rail. Load a founder-approved, hand-curated list of 30–50
**face-is-brand professionals** (realtors, recruiters, coaches) via a
small CSV loader over the existing canonical write-through.

The sender is a **new, dedicated LinkedIn account carrying the
founder's real identity** (profile photo generated with magikshot
itself). It connects through the real hosted-auth flow on **day one**
— proving connect, webhook delivery, and tenant binding weeks before
the first send — then warms up manually for 2–3 weeks. Only after
warm-up does the founder start the campaign: a hand-written
connection note plus DM follow-ups, sent under the built-in warm-up
pacer, observed daily against a runbook with seven explicit pass
criteria. A genuine prospect reply is the real win.

## User Stories

1. As the magikshot founder, I want magiklead to run its first real
   LinkedIn campaign for magikshot, so that the product is validated
   on its own outreach before any external customer depends on it.
2. As the operator, I want the whole smoke to run from my laptop's
   devpod stack, so that I don't have to build and pay for a
   production deployment first.
3. As the operator, I want a dedicated smoke devpod separate from my
   daily dev pod, so that a routine seed or reset can never wipe a
   live campaign's state.
4. As the operator, I want the smoke pod's database dumped nightly,
   so that weeks of real campaign state survive a laptop mishap.
5. As the magikshot founder, I want the sender to be a new dedicated
   LinkedIn account with my real identity as founder, so that the
   outreach is honest and my personal account carries no automation
   risk.
6. As the magikshot founder, I want the new account's profile photo
   generated with magikshot, so that the product demonstrates itself
   on every profile visit.
7. As the operator, I want a written warm-up protocol for the new
   account (manual activity, connections, cadence, duration), so that
   it looks and behaves like a real account before automation touches
   it.
8. As the operator, I want the new account connected through the real
   hosted-auth flow on day one, so that connect, webhook delivery,
   and tenant binding are proven weeks before the first send.
9. As the operator, I want campaign start to remain a manual,
   explicit action, so that nothing sends until warm-up is complete
   and I say go.
10. As the operator, I want a stable tunnel hostname on the
    magikshot.com zone pointing at the smoke pod's API, so that
    Unipile webhooks reach my laptop for the whole smoke without
    re-registration churn.
11. As the operator, I want the tunnel supervised as a service, so
    that it comes back by itself after a reboot.
12. As the operator, I want the three Unipile webhook sources —
    account status, relations, messaging — registered once against
    the stable hostname with the shared auth header, so that status
    changes, acceptances, and replies flow in live.
13. As the operator, I want an idempotent script to list, register,
    and prune those webhook registrations, so that recovering from a
    dead registration is one command instead of dashboard
    archaeology.
14. As the operator, I want the smoke pod's public base URL
    configurable while every other pod keeps the local default, so
    that hosted-auth links carry a reachable notify address only
    where intended.
15. As the operator, I want the previously exposed Unipile API key
    rotated and my personal LinkedIn account disconnected at Unipile,
    so that the smoke starts from clean credentials and a clean
    account slate.
16. As the magikshot founder, I want the first list to be 30–50
    face-is-brand professionals (realtors, recruiters, coaches), so
    that the pitch is native to LinkedIn and reply signal arrives
    fast.
17. As the magikshot founder, I want to review and approve the
    curated list before it is loaded, so that no real person is
    contacted without my sign-off.
18. As the operator, I want a loader that takes the approved list as
    a CSV and plants it in the canonical person graph, so that
    prospects appear in lead search like data from any other source.
19. As the operator, I want the loader to be idempotent and to report
    exactly what it planted, updated, or skipped, so that re-runs are
    safe and auditable.
20. As a magiklead user, I want people loaded by *any* source to
    carry LinkedIn identifiers the search and send rail actually
    read, so that a loaded prospect can always be found and messaged.
21. As the magikshot founder, I want plays generated from
    magikshot.com's website analysis, so that the campaign is
    anchored on the product's real positioning.
22. As the magikshot founder, I want to hand-write and approve the
    connection note and DM copy before the campaign can start, so
    that real people only receive words I stand behind.
23. As the operator, I want sends governed by the built-in warm-up
    pacer and weekly caps, so that the fresh account ramps gently
    instead of tripping LinkedIn's defenses.
24. As the operator, I want acceptances to advance leads to the first
    DM automatically — via webhook or the reconcile poll — so that
    sequences progress even when LinkedIn's notifications lag by
    hours or the laptop was asleep.
25. As the magikshot founder, I want a prospect's reply to halt their
    sequence immediately and suppress them across campaigns, so that
    nobody who answered ever receives an automated follow-up.
26. As the operator, I want the acceptance-rate breaker to pause the
    account when accepts run low — and to see that state clearly — so
    that a struggling account is protected and the pause reads as the
    product working.
27. As the operator, I want stale invites withdrawn automatically on
    the built-in schedule and never by hand, so that cleanup happens
    without burning recipients (LinkedIn penalizes rapid re-invites).
28. As the operator, I want a runbook covering the day-0 checklist,
    the connect ceremony, the approval gates, a daily observation
    routine, and the pass criteria, so that any future session can
    run and judge the smoke without re-deriving it.
29. As the operator, I want every pass criterion verifiable from
    logs, database state, or the UI, so that the verdict is
    evidence-based rather than vibes.
30. As the operator, I want a clean disconnect at the end that
    revokes the Unipile session and unbinds the account in-app, so
    that the smoke leaves no dangling access.
31. As the magikshot founder, I want at least one genuine prospect
    reply recorded in-app, so that the smoke validates the pitch and
    the plumbing at once.
32. As a magiklead developer, I want everything the smoke produces —
    loader, scripts, fixes, runbook — committed production-grade, so
    that the eventual real deployment reuses it unchanged.

## Implementation Decisions

- **Execution environment.** The smoke runs in a dedicated devpod on
  the laptop, on its own worktree and database. The founder uses the
  local https frontend as any customer would. No production
  deployment is created.
- **Identifier unification (bugfix).** Every writer of LinkedIn
  identity (ingest resolution, the new curated loader, dev seed, any
  future live search) and every reader (lead-search attachment, the
  invite and DM due-lead queries) converge on the send rail's
  existing identifier type, storing the full canonical profile URL.
  A single shared normalization helper defines that canonical form;
  a data migration rewrites rows previously written under the
  divergent type. After this, "LinkedIn prospect" means the same
  thing everywhere.
- **Curated prospect loader.** A thin CLI over the existing canonical
  write-through (organization → person → identifier → employment).
  Input: CSV with name, first/last, title, company, company domain,
  LinkedIn profile URL, location. It validates and normalizes each
  row, upserts idempotently keyed on the profile identifier, touches
  nothing outside its input, and prints a planted/updated/skipped
  report. It runs inside the pod through the standard exec path.
- **Webhook plumbing.** A named cloudflared tunnel with a hostname on
  the magikshot.com Cloudflare zone fronts the smoke pod's API,
  supervised as a user service. The three Unipile webhook sources
  (account status, relations, messaging) are registered through
  Unipile's API against that hostname, carrying the static auth
  header the webhook endpoint verifies (Unipile has no body signing).
  Registration is wrapped in an idempotent checked-in script with
  list, register, and prune modes. The pod's public base URL becomes
  a parameterized value whose default remains the local https
  literal, so only the smoke pod advertises the tunnel.
- **Connect and bind.** Day-one hosted-auth connect from the smoke
  UI. Success means the connected-account event arrives through the
  tunnel and binds tenant and account with zero manual SQL — the one
  thing local development could never prove. Per the spike's
  learnings, the reconcile poll is treated as the authoritative
  recovery path for acceptances and replies; webhooks are the fast
  path, not the only path.
- **Campaign mechanics — untouched.** The existing pacer (week-one
  daily ramp, weekly cap, acceptance-rate breaker), the reconcile
  loop, the stale-invite withdrawal, reply-halt and cross-campaign
  suppression all run as shipped. The sequence is a step-0 connection
  note within the note character limit plus at least one DM. Plays
  come from website analysis of magikshot.com; the note and DM copy
  are hand-authored.
- **Human gates.** Three explicit approvals: the curated list before
  loading, the copy before the campaign exists, and campaign start
  itself — all by the founder. Everything else runs unattended.
- **Hygiene.** Rotate the Unipile API key that leaked into a session
  transcript; disconnect the founder's personal LinkedIn at Unipile
  so the new account is the only connected one. Nightly dump of the
  smoke pod's database. The smoke pod is never seeded or destroyed
  for the duration of the run.

## Testing Decisions

- A good test observes **external behavior**: what a reader query
  returns, what the report says, what an endpoint does — never
  internal structure. Tests should keep passing through refactors
  that preserve behavior.
- **Identifier unification** gets unit tests: the normalization
  helper across messy real-world inputs (scheme variants, trailing
  slashes, query strings, uppercase slugs), the data migration over
  legacy-shaped rows, and assertions that people written by each
  source are visible to the lead-search attachment and to the invite
  and DM due-lead queries afterwards.
- **The loader** gets an integration test against the devpod
  database — prior art: the Unipile connect-and-bind integration
  suite, which runs inside the pod against its live schema. Load a
  fixture CSV, assert the full graph lands; run it again, assert
  idempotency; include malformed rows, assert per-row rejection with
  an accurate report; assert loaded people are visible to the rail's
  queries.
- **The webhook script** gets a dry-run mode test asserting the
  registrations it *would* create, and a happy-path test against a
  stubbed endpoint.
- The smoke itself is judged by the runbook's pass criteria, each
  verified from logs, database state, or the UI. The existing e2e
  crafted-payload suite continues covering the webhook spine in CI;
  the smoke adds the live-traffic proof CI cannot.

## Out of Scope

- The Hetzner production deployment and everything in the release
  phase (VPS, Sentry, Resend, sender-domain auth, backups, uptime,
  key swaps, public launch). The smoke must not depend on any of it.
- Wiring the dormant live LinkedIn search (RapidAPI) into lead
  search — the smoke uses the curated list only.
- The email rail entirely: Gmail OAuth sends, deliverability, and
  the unsubscribe mailto-channel gap.
- Registering the magiklead.com domain or any production DNS.
- Multi-account support beyond the free tier's single connected
  account, and any send-window / timezone / business-hours gating.
- Production Clerk or Stripe key swaps — dev keys pass startup
  validation and suffice locally.

## Further Notes

- **Critical path is calendar, not code.** Account creation, the
  day-one connect, and the warm-up clock (2–3 weeks) start
  immediately; the code (identifier fix, loader, wiring, runbook)
  lands comfortably within that window.
- **Expected behaviors that are not failures:** LinkedIn's
  acceptance notifications can lag ~8 hours (the reconcile poll
  covers this); laptop downtime just pauses sending — the pacer
  resumes and reconcile back-fills; the acceptance breaker pausing a
  fresh account on a small sample is plausible and means the
  protection works.
- **Spike learnings honored:** webhook auth is a static header (no
  body signing exists); never withdraw invites by hand — re-inviting
  a recipient soon after returns a recipient-specific rate-limit
  that burns them for weeks; treat the reconcile poll, not the
  webhook, as the path that must work.
- **Costs:** the Unipile subscription is already active (minimum
  tier covers up to 10 accounts); the tunnel and DNS are free on the
  existing Cloudflare zone; Anthropic usage for website analysis and
  plays is negligible. No new spend beyond what exists today.
- **Success** is all seven runbook criteria green — webhook-bound
  connect with zero manual SQL, loaded prospects searchable, campaign
  created from approved copy, pacing held, acceptance advances to DM,
  reply halts and suppresses, clean disconnect — plus at least one
  genuine reply. The smoke doubles as the dress rehearsal for the
  real production launch: everything it builds carries over.
