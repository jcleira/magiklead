# LinkedIn-Only Outreach — Product Requirements

**Date:** 2026-06-09
**Status:** Draft — product shape + implementation design resolved (two /code-research passes)

---

## Problem Statement

MagikLead's lead sourcing is too expensive to run cold outreach at
scale. Today the synchronous lead search runs on People Data Labs
(PDL) as the primary source, and at scale the real cost is roughly
**$0.29 per lead-with-email** — about ten times too expensive, so the
data cost of a single campaign can exceed the customer's revenue.

The root cause is structural: PDL bills per returned record, which
makes both the *list* of people and their *emails* expensive, and
every email reached requires a separate find-and-verify step that is
either unreliable (homegrown SMTP probing, which Hetzner's default
outbound port-25 block defeats and the canonical graph explicitly
forbids because guessed emails bounce) or a paid lookup.

The deeper realization from research: the entire expense lives in the
*email channel*. If outreach happens over LinkedIn instead, the
LinkedIn profile URL **is** the address — there is no email to find,
verify, or pay for. The variable per-lead data cost disappears and is
replaced by a flat per-connected-account fee.

## Solution

Pivot the outbound channel to **LinkedIn direct messages, only.** A
customer connects their own LinkedIn account (the same shape as the
existing "connect your Gmail" flow), MagikLead sources prospects as
LinkedIn profiles, and the campaign engine sends connection requests
and follow-up DMs on the customer's behalf, halting the sequence the
moment a prospect replies.

The send rail is **Unipile**, an API-first unified-messaging provider
that exposes a real REST API and a hosted authentication wizard for
connecting each tenant's LinkedIn account, runs inside the user's real
authenticated session, and charges a flat per-connected-account fee
with no per-message cost. Sourcing writes prospects through to the
**existing canonical person graph** keyed on the LinkedIn URL, so the
data caches across tenants for free exactly as PDL results did — but
without any email step.

Because LinkedIn caps connection requests at roughly 100–150 per
account per week and a freshly connected account needs a multi-week
warmup, throughput is governed by **connected accounts (seats), not by
leads.** Capacity is metered and priced per connected LinkedIn account.

From the customer's point of view: connect LinkedIn, describe the
ideal customer, review the prospects MagikLead found, approve a
message sequence, and watch accepted connections turn into
conversations — with zero per-lead data cost and no email deliverability
to manage.

## User Stories

### Connecting and managing a LinkedIn account

1. As a tenant user, I want to connect my own LinkedIn account through
   a guided hosted flow, so that MagikLead can send on my behalf without
   me sharing my password directly with MagikLead.
2. As a tenant user, I want the LinkedIn connect flow to look and behave
   like the existing Gmail connect flow, so that the product feels
   consistent.
3. As a tenant user, I want to see the live status of each connected
   LinkedIn account (connected, warming up, restricted, disconnected),
   so that I know whether outreach can run.
4. As a tenant user, I want to connect more than one LinkedIn account
   (mine and teammates'), so that I can raise my weekly outreach
   capacity beyond a single account's cap.
5. As a tenant user, I want to disconnect a LinkedIn account, so that I
   can stop all automation on that identity immediately.
6. As a tenant user, I want to be shown the risk of automating my real
   LinkedIn identity and explicitly consent before connecting, so that I
   am making an informed decision about my professional account.
7. As a tenant user, I want to see how much of this week's connection-
   request capacity remains on each account, so that I can plan campaign
   volume.
8. As a tenant user, I want a newly connected account to ramp its
   sending volume gradually over the first weeks, so that it is less
   likely to be flagged or restricted by LinkedIn.

### Sourcing prospects

9. As a tenant user, I want to describe my ideal customer (titles,
   industries, company size, locations, free-text ICP) and get back a
   list of matching LinkedIn prospects, so that I do not have to find
   people manually.
10. As a tenant user, I want each prospect to arrive with name, title,
    company, and LinkedIn profile, so that I can judge fit before
    reaching out.
11. As a tenant user, I want sourcing to cost me nothing per prospect,
    so that data cost never exceeds the value of a campaign.
12. As an operator, I want sourced prospects written into the canonical
    person graph keyed on their LinkedIn URL, so that a prospect found
    once is cached for every tenant and never re-fetched needlessly.
13. As an operator, I want a demo-mode sourcing path that scrapes
    company websites with no external dependency, so that the product
    can be demonstrated without a paid data provider.
14. As an operator, I want a production sourcing path backed by a cheap
    LinkedIn people-search API, so that prospect lists are broad enough
    to fill real campaigns.
15. As an operator, I want the same prospect appearing from two sources
    to resolve to one canonical person, so that the graph does not
    accumulate duplicates.

### Building and running a sequence

16. As a tenant user, I want to save a sourced prospect to my saved-lead
    list, so that I can curate who to contact.
17. As a tenant user, I want to add saved prospects to a campaign, so
    that they enter an outreach sequence.
18. As a tenant user, I want to author a connection-request note plus a
    series of follow-up DMs with delays between them, so that I control
    the message and cadence.
19. As a tenant user, I want personalization tokens (first name,
    company, title) in my messages, so that outreach does not read as a
    blast.
20. As a tenant user, I want the campaign to send a connection request
    first and only DM after the prospect accepts, so that messaging
    follows how LinkedIn actually works.
21. As a tenant user, I want follow-up DMs to send automatically on
    their schedule until the prospect replies, so that I do not have to
    chase manually.
22. As a tenant user, I want a prospect's reply to immediately halt all
    further messages to them, so that I never message someone who is
    already in conversation.
23. As a tenant user, I want sends paced within safe per-day and per-week
    limits per account, so that volume never spikes in a way that risks
    the account.
24. As a tenant user, I want the campaign to pause an account
    automatically if its acceptance rate drops too low, so that a bad
    list does not get the account restricted.
25. As a tenant user, I want prospects who never accept the connection
    request to drop out of the sequence after a set window, so that
    stale invites do not pile up.

### Replies, suppression, and safety

26. As a tenant user, I want a unified inbox view of prospect replies
    per campaign, so that I can continue conversations.
27. As a tenant user, I want a prospect who asks to stop to be added to
    my suppression list, so that they are never contacted again across
    any campaign.
28. As an operator, I want suppression to work for prospects who have no
    email (LinkedIn-only), so that "do not contact" is enforced by
    person identity, not just by email address.
29. As an operator, I want the "zero messages to replied prospects"
    invariant enforced by the same suppression gate the email engine
    uses, so that the guarantee is single-sourced.
30. As an operator, I want every outreach event (invite sent, accepted,
    DM sent, follow-up sent, replied, skipped, failed) recorded, so that
    metrics and the halt guarantee are auditable.

### Metrics and capacity

31. As a tenant user, I want per-campaign metrics (invites sent,
    acceptance rate, DMs sent, reply rate), so that I can judge what is
    working.
32. As a tenant user, I want to see my total weekly new-prospect
    capacity across all connected accounts, so that I understand the
    ceiling I am working within.
33. As an operator, I want capacity metered per connected account so
    that plan limits map to seats, so that pricing reflects the real
    cost driver.
34. As a tenant on the free tier, I want one connected LinkedIn account,
    so that I can try the product before paying.

### Reliability and operations

35. As an operator, I want inbound LinkedIn events (acceptances,
    replies, account-status changes) delivered by webhook and verified
    by signature, so that the system reacts in near real time without
    polling.
36. As an operator, I want a restricted or disconnected account to pause
    its campaigns and surface a clear status to the user, so that sends
    do not silently fail.
37. As an operator, I want the Unipile integration isolated behind a
    clean seam, so that a second provider could be swapped in if Unipile
    is disrupted.
38. As an operator, I want a hard cap on outreach actions per account
    enforced server-side regardless of campaign configuration, so that no
    misconfiguration can exceed safe limits.

## Implementation Decisions

### Channel and provider

- The product's outbound channel for this release is **LinkedIn DMs
  only**. The existing Gmail send path is left intact and untouched but
  is not exercised by LinkedIn campaigns.
- **Unipile** is the send rail: per-tenant account connection via its
  hosted-auth wizard, plus send-invitation, send-message, and
  send-InMail operations, plus inbound events by webhook. This mirrors
  the existing per-tenant Gmail OAuth connect/store/send architecture.

### New deep modules

- **`linkedin/unipile`** — encapsulates all Unipile API access behind
  an HTTP-client seam (the same seam pattern as the `leads/pdl`
  module). Interface surface: generate a hosted-auth link for a tenant;
  register a connected account from the auth callback/webhook; send a
  connection invitation; send a DM in a chat; send an InMail; and parse
  inbound webhook event payloads into typed events. Stubbable in tests
  with no network.
- **`linkedin/pacer`** — the warmup-and-cap policy engine, and the
  module to extract and test hardest. Pure decision logic: given a
  connected account's warmup state, rolling per-day and per-week
  counters, current acceptance rate, and a clock, it returns how many
  connection invitations and DMs that account may send right now. It
  owns the weekly invitation ceiling, the per-day sub-cap, the
  multi-week ramp curve, randomized-delay guidance, and the
  acceptance-rate circuit breaker. No I/O beyond reading counters. The
  chosen "Standard" preset: 100 invites/week ceiling, warmup ramp from
  ~8/day in week 1 to ~20/day by week 4, withdraw still-pending invites
  after 21 days, and pause an account when its trailing-7-day acceptance
  rate falls below 20%.
- **`leads/linkedin_search`** — production sourcing via a cheap
  RapidAPI LinkedIn people-search, writing each result through to the
  canonical graph keyed on the LinkedIn URL. This mirrors the
  `leads/pdl` search-plus-write-through contract, minus any email
  handling. The existing website-scraper is repurposed as the demo-mode
  source feeding the identical write-through.

### Modified modules

- **Worker** — gains a new LinkedIn sequence tick running alongside the
  existing Gmail sender tick. For each due campaign lead it asks the
  pacer for an allowance, sends the next sequence step (invite or
  follow-up DM) through `linkedin/unipile`, and advances the lead's
  sequence state. A new webhook-driven ingest path classifies
  invitation-accepted and inbound-message events.
- **Suppression** — its "do not contact" decision and recording are
  extended from an email key to also key on canonical person identity
  (person_id / LinkedIn URL), so that emailless LinkedIn prospects can
  be suppressed and reply-halted through the same single source of
  truth. The existing email-keyed behavior is preserved for the Gmail
  engine.
- **Canonical write-through** — reuses the existing persons /
  organizations / employments / person_identifiers upsert contract. The
  LinkedIn source populates `person_identifiers` with a `linkedin_url`
  identifier as the dedup anchor and writes no `emails` rows.
- **Frontend** — a LinkedIn connect-status surface in settings (clone of
  the Gmail connect UI, including the risk-consent step), a campaign
  sequence builder for LinkedIn steps (invite note plus delayed
  follow-up DMs), and metrics/capacity displays (acceptance rate, reply
  rate, remaining weekly capacity per account).

### Sequence execution model (event-gated)

Email sequences are purely time-gated: each step fires on a timer. A
LinkedIn sequence must **stop and wait for the prospect to accept the
connection** before any DM can be sent, so the engine is a hybrid of
timer-driven sends and event-driven transitions, reusing the existing
`campaign_leads` tick rather than adding a parallel one:

- The connection invitation is **step 0**, gated by the pacer. On send,
  the lead moves to `awaiting_accept` and its `next_send_at` is cleared
  — there is no timer, because acceptance is unbounded (it may take days
  or never come).
- The **invitation-accepted event** (Unipile webhook, backstopped by the
  reconciliation poll) flips the lead to `active`, sets
  `next_send_at = NOW()` and `current_step = 1`, so the existing
  60-second sender tick sends the first DM on its next pass.
- DM steps 1+ advance on `delay_days` timers exactly like the email
  engine (same `current_step` / `next_send_at` advance, same jitter).
- An **inbound-message event** halts the sequence (reply path); an
  invitation still pending past the pacer's withdrawal window is
  withdrawn and the lead closed `not_accepted`.

The existing sender selection — `WHERE next_send_at <= NOW() AND
status='active'` — is reused unchanged; the only new state is the
`awaiting_accept` status and the webhook transition back into `active`.

### Schema changes

- `campaigns.linkedin_sequence` (JSONB) **already exists** alongside the
  email `sequence`; this release populates and reads it. A LinkedIn
  sequence step is `{ step, delay_days, body }` — no subject (DMs have
  none); step 0's body is the connection note (LinkedIn's ~300-char
  limit applies), steps 1+ are DMs. The existing `{{first_name}}` /
  `{{company}}` / `{{title}}` personalization tokens are reused.
- New `campaigns.channel` column (`email` | `linkedin`) routes a
  campaign to the correct worker tick. Existing campaigns default to
  `email`.
- **`campaign_leads`** reuses the existing sequence-state columns
  (`status`, `current_step`, `next_send_at`, `last_sent_at`,
  `last_replied_at`, `person_id`); the only additions are
  `linkedin_account_id` (the account that sent the invite — the DM must
  come from the same identity), `linkedin_invitation_id` and
  `linkedin_chat_id` (mapping inbound webhooks back to the lead), and
  `accepted_at`. The `status` enum gains `awaiting_accept` and
  `not_accepted`.
- New **`linkedin_accounts`** table: tenant, `unipile_account_id`,
  status (`connecting` | `active` | `warming` | `restricted` |
  `disconnected`), `warmup_started_at`, rolling weekly + daily invite
  counters with their window starts, cached 7-day acceptance rate, and
  last error. Shaped after the existing `gmail_accounts` table, minus
  OAuth tokens (Unipile holds the session).
- New **`linkedin_events`** table mirroring `email_events`: keyed to a
  campaign lead, event type (`invite_sent`, `accepted`, `dm_sent`,
  `replied`, `skipped`, `failed`, `withdrawn`), the Unipile message/chat
  id, and a timestamp. Drives metrics and the "zero DMs to replied
  prospects" invariant.
- **`unsubscribes`** gains a nullable `person_id` so suppression works
  for emailless LinkedIn prospects; `IsSuppressed` gains a person-keyed
  path. Only the `reply` and `manual` reasons apply on LinkedIn (no
  bounces).
- `person_identifiers` already supports a `linkedin_url` type; reused
  with no change. A generic `FindPersonByIdentifier(type, value)` query
  is added (the PDL path hard-codes `pdl_id`).

### API contracts and configuration

- A tenant-scoped endpoint returns a Unipile hosted-auth URL to begin
  connecting an account. Because Unipile owns the redirect (unlike the
  Gmail callback we host), the tenant + connecting-user binding rides as
  a signed `pkg/jwt` token in the Unipile account metadata, verified on
  the `account.connected` webhook — rather than as state on our own
  callback.
- A public, signature-verified `/webhooks/unipile` endpoint ingests
  account-status changes, invitation acceptances, and inbound messages,
  idempotently (mirrors the Clerk/Stripe handlers — 503 if the secret
  is unconfigured, never silently accept). A low-frequency (~30–60 min)
  reconciliation poll backstops dropped webhooks so no lead strands in
  `awaiting_accept`.
- A tenant endpoint lists connected LinkedIn accounts with status,
  warmup state, and remaining weekly capacity.
- Lead search reuses the existing search endpoint; the LinkedIn source
  populates the canonical and the email requirement is dropped.
- Campaign endpoints define a LinkedIn step sequence (connection note
  plus ordered follow-up DMs with delays) on `campaigns.linkedin_sequence`.
- Config follows the existing optional-degrade idiom: `UNIPILE_API_KEY`
  (absent → LinkedIn features disabled, like PDL), `UNIPILE_DSN` (the
  per-tenant Unipile subdomain), and `UNIPILE_WEBHOOK_SECRET` (required
  once the key is set, else the webhook returns 503).

### Capacity and pricing model

- Capacity is governed by **connected LinkedIn accounts**, each capped
  near 100–150 new connection requests per week after a multi-week
  warmup. **MVP ships one connected account per tenant across all
  tiers** — multi-account connection and cross-account pooling are
  deferred (see Out of Scope).
- The per-account weekly cap is set by LinkedIn on the account itself
  and **cannot be bought up** — neither Premium/Sales Navigator nor a
  higher Unipile spend raises it. The consequence: in MVP there is no
  "more LinkedIn volume" to sell on a single account, so either paid
  tiers sell other value (more campaigns, sourcing volume, analytics) or
  multi-account becomes the first fast-follow that switches on per-seat
  pricing. A pricing decision to settle before launch, not a build
  blocker.

## Testing Decisions

Good tests here assert **external behavior** — database state
transitions, the classification of inbound events, the payloads sent to
a stubbed Unipile, and the policy decisions of the pacer — never
internal implementation details. The codebase's existing
provider-seam and worker tests are the prior art to follow.

- **`linkedin/pacer`** (highest value) — exhaustive unit tests of the
  allowance decision across the warmup ramp, the per-day and per-week
  caps, the acceptance-rate circuit breaker, and counter-window rollover,
  driven by an injected clock and counter inputs. Pure logic, no I/O.
- **`linkedin/unipile`** — table-driven tests against an in-process HTTP
  stub (the pattern used by the PDL module tests): assert the correct
  request shape for invite, DM, and InMail; assert status mapping for
  rate-limit, auth-failure, and account-restricted responses; assert
  inbound webhook payloads parse into the right typed events.
- **Worker LinkedIn tick and webhook ingest** — behavioral tests in the
  style of the existing poller tests: an invitation-accepted event
  advances the lead and triggers the first DM; an inbound message flips
  the campaign lead to replied, writes a replied event with the inbound
  message id, and the "zero DMs to replied prospects" invariant holds
  through the suppression gate.
- **`leads/linkedin_search` write-through** — tests mirroring the PDL
  write-through tests: one search record lands persons / organizations /
  employments / a `linkedin_url` identifier, re-ingesting the same record
  dedups to one canonical person, and no `emails` row is created.
- **Suppression person-key** — extend the existing suppression tests to
  cover suppression and reply-halt keyed on person identity for prospects
  with no email.

The user has approved testing the deep modules above; the shallow
handlers and frontend surfaces are covered by the existing end-to-end
suite rather than dedicated unit tests.

## Out of Scope

- Any change to the email / Gmail send, reply, bounce, or unsubscribe
  paths. They remain intact and are simply not used by LinkedIn
  campaigns.
- PDL: no new PDL work. The module remains in the tree, unused by this
  release; it is effectively demoted by no longer being the search
  spine.
- Email finding of any kind — the homegrown SMTP pattern-guesser and any
  third-party email finder/verifier are not part of a LinkedIn-only
  product.
- Removal of the legacy flat `leads` table and the old asynchronous
  discover pipeline. They are superseded by the canonical graph, but
  deleting them is a separate cleanup, not a prerequisite here.
- Stripe price/plan changes for per-seat billing. Capacity is metered by
  connected account in this release; the paid-plan price wiring is a
  fast-follow.
- Multi-account connection per tenant, cross-account capacity pooling,
  and the round-robin/least-loaded assignment that goes with them —
  deferred to the first fast-follow.
- Other channels (Outlook, custom SMTP, WhatsApp) and the LinkedIn
  InMail lane (InMail needs a Sales Navigator seat and caps at ~50/mo;
  it is a scarce premium lane, not the primary sequence).

## Further Notes

### Risks

- **ToS and account-restriction risk.** Automating outreach on a
  customer's real LinkedIn account violates LinkedIn's User Agreement,
  and the asset at stake is the customer's actual professional identity,
  not a disposable sending domain. A March 2026 LinkedIn enforcement
  wave restricted a large share of cloud-automation accounts and removed
  at least one major vendor's presence entirely. Mitigations are built
  into the design — the pacer's warmup ramp, per-day and per-week caps,
  randomized delays, and acceptance-rate circuit breaker; Unipile's
  real-session architecture; a dedicated IP per account where available;
  and explicit customer risk-consent at connect time — but it is never
  truly "safe." This must be communicated honestly to customers.
- **Throughput ceiling.** One connected account reaches on the order of
  100–150 new prospects per week and is not at full volume for the first
  three to four weeks. Product capacity scales by adding connected
  accounts, not by spending more. Plan limits and customer expectations
  must be set against this ceiling.
- **Single-vendor dependency.** The connect, send, and inbound paths all
  ride on Unipile; a disruption to Unipile (including any action LinkedIn
  takes against it) stops outreach. The `linkedin/unipile` seam is kept
  deliberately clean so an alternative provider could be substituted.

### Compliance

- LinkedIn DMs are not email, so US CAN-SPAM does not apply, but EU
  ePrivacy/GDPR considerations do. B2B outreach generally rests on a
  legitimate-interest basis with an honored opt-out (a "stop" reply
  routes to suppression). Germany is notably stricter and may warrant
  special handling.

### Sourcing demo vs production

- Demo mode sources prospects by scraping company websites (no external
  dependency), and production mode sources them from a cheap RapidAPI
  LinkedIn people-search. Both write through the canonical graph
  identically, so switching between them is a source-selection setting,
  not a code path the rest of the system can observe.
