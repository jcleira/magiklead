# PRD — Finish the LinkedIn (Unipile) integration

## Problem Statement

An operator connects a LinkedIn account and launches a LinkedIn
outreach campaign expecting the product to do the obvious thing: send
connection requests at a safe pace, automatically send a first message
the moment someone accepts, follow up on schedule, and stop the instant
a prospect replies — never contacting that person again.

Today only the outbound half works. The inbound half is broken:

- **Acceptances are never detected**, so the first message never fires
  and the whole sequence stalls after the invite.
- **The product can mistake its own outgoing messages for a prospect
  reply**, wrongly halting the sequence and permanently suppressing a
  person who never actually replied.
- **Connecting an account may not actually tie it to the workspace** —
  during the trial the binding had to be done by hand in the database.
- **Invites/messages are addressed with a profile URL**, but Unipile
  needs an internal member id, so worker-driven sends fail.

Worst of all, the automated tests are green but **untrustworthy**: they
were written against *guessed* Unipile message shapes that don't match
what Unipile really sends, so passing tests prove the wrong contract.

## Solution

Make the full lifecycle work against **real Unipile**, and rebuild the
tests on **real captured Unipile messages** so that green means it
genuinely works.

The operator's experience after this work:

- Connect a LinkedIn account in the app and have it linked to the
  workspace automatically, with no manual database editing.
- Have invites and messages reach the right person, because the product
  resolves each prospect's profile to the member id Unipile needs —
  once per person, remembered permanently.
- Have acceptances noticed within ~45 minutes (by checking the account's
  real connections, not by waiting on a notification that can lag eight
  hours), which auto-fires the first message.
- Have follow-ups go out on schedule, and have the sequence stop the
  instant a prospect *actually* replies — correctly telling the
  operator's own messages apart from the prospect's.
- Have a restricted or disconnected account stop sending and surface for
  reconnect.

The work is gated by a **validation spike**: capture the real Unipile
messages first, then build and test everything against those captures.

## User Stories

1. As an operator, I want to connect my LinkedIn account through the app
   and have it immediately linked to my workspace, so that I can run
   campaigns without anyone hand-editing the database.
2. As an operator, I want the account to show a clear connected/active
   status after connecting, so that I know it is ready to send.
3. As an operator, if my connection carries expired or bad credentials,
   I want the account to surface the real error, so that I know to retry.
4. As an operator, I want the product to translate a prospect's LinkedIn
   profile into the identifier Unipile needs, so that my invites and
   messages actually reach the right person.
5. As an operator, I want that translation to happen only for people I
   actually contact, so that we never waste lookups (or risk rate limits)
   on prospects I never message.
6. As an operator, I want a prospect's resolved identifier remembered
   permanently on that person, so that the same person is never looked
   up twice, even across different campaigns.
7. As an operator, if a prospect's profile can't be resolved because it
   is deleted or private, I want that lead marked failed rather than
   retried forever, so that my queue keeps moving.
8. As an operator, if a lookup fails because of a temporary glitch, I
   want it retried automatically on the next tick, so that a transient
   hiccup doesn't kill an otherwise-good lead.
9. As an operator, I want invites sent within safe LinkedIn limits
   (daily/weekly caps and the warmup ramp), so that my account isn't
   flagged.
10. As an operator, I want sending to pause automatically if my
    acceptance rate craters, so that I don't burn the account on a bad
    list.
11. As an operator, I want the product to notice within about 45 minutes
    when someone accepts my connection request, so that the follow-up is
    timely.
12. As an operator, I want acceptance detected by checking my account's
    real connections, so that it never depends on a notification that
    can lag up to eight hours.
13. As an operator, I want acceptance detection to be safe to run
    repeatedly without acting twice, so that a prospect is never messaged
    twice for one acceptance.
14. As an operator, I want the first message sent automatically the
    moment an acceptance is detected, so that I strike while the new
    connection is fresh.
15. As an operator, I want follow-up messages sent on schedule after the
    first, so that I keep the conversation going without manual work.
16. As an operator, I want every message to reuse the already-resolved
    identifier, so that no extra lookups happen mid-sequence.
17. As an operator, I want the sequence to stop the instant a prospect
    replies, so that I never auto-message someone who is already
    engaging.
18. As an operator, I want the product to correctly tell my own outgoing
    messages apart from the prospect's replies, so that it never halts a
    sequence on my own message.
19. As an operator, I want a prospect who replied to be excluded from all
    my other campaigns too, so that they are never double-contacted.
20. As an operator, I want replies recognized promptly via Unipile's
    message notification, with the periodic check as a backup, so that I
    rarely keep messaging after a reply.
21. As an operator, if LinkedIn restricts my account, I want sending to
    stop and the account flagged, so that I can reconnect before doing
    damage.
22. As an operator, if my account disconnects, I want it shown as
    disconnected, so that I know to reconnect it.
23. As an operator, after I reconnect a flagged account, I want sending
    to resume, so that my campaigns continue.
24. As an operator, I want stale unaccepted invites withdrawn after the
    set horizon, so that my pending-invite count stays healthy.
25. As an operator, I want the capacity/outcomes dashboard (sent,
    accepted, replied) to reflect reality, so that I can see how the
    campaign is performing — which depends on the inbound events being
    recorded correctly.
26. As a developer, I want the integration built against real captured
    Unipile messages, so that it matches what Unipile actually sends
    rather than what we assumed.
27. As a developer, I want tests that feed real captured messages and
    assert real outcomes, so that a green suite proves the integration
    genuinely works.
28. As a developer, I want the webhook authenticated exactly the way
    Unipile authenticates (a fixed secret header), so that real Unipile
    calls are accepted and forgeries are rejected.
29. As a developer, I want the imagined-payload tests deleted, so that no
    test continues to prove a contract that doesn't exist.
30. As a developer, I want the temporary capture log removed once
    fixtures are frozen, so that we never log raw webhook bodies in
    production.
31. As a developer, I want payload parsing, the webhook→database
    lifecycle, the member-id resolver, and the accept matcher each
    tested in isolation, so that a failure points straight at the broken
    piece.
32. As an admin, I want the leaked Unipile API key rotated, so that the
    integration is not running on an exposed credential.
33. As a developer, I want the connect flow's signed-metadata token to
    arrive within its validity window, so that automatic binding doesn't
    silently fail on a slow echo.

## Implementation Decisions

### Validation spike (precursor, gates everything else)

- Before any parser, handler, or test is crafted, capture the **real**
  Unipile messages and build against them. No assumed shapes.
- The spike runs against the live trial account over a fresh tunnel,
  using the already-present temporary capture log, and records:
  - the **account-connected** message (the headline unknown — confirm it
    auto-binds the account to the tenant with **zero** manual SQL);
  - **account-status** transitions emitted during connect/sync;
  - an inbound **message** and an outbound **message** (both directions)
    — the bytes that validate reply-direction logic;
  - the **connections list**, **sent-invitations list**, and **chats
    list** responses the periodic check reads.
- The captured bytes are frozen as the test fixtures. The temporary
  capture log is removed only after fixtures are frozen.
- The `new_relation` acceptance notification is **not** captured on
  demand (its eight-hour lag plus invite rate limits make that
  impractical) and is **not** required, because acceptance is detected
  by the connections check instead.

### Member-id resolution and caching (deep module: the resolver)

- Unipile needs a resolved LinkedIn **member id** (not the profile URL)
  to send anything and to recognize an accepter.
- The member id is treated as **permanent person data**: it is cached on
  the person via a `person_identifiers` row of type `linkedin_member_id`
  (no new per-lead column). The existing `linkedin_url` identifier is the
  resolution input.
- Resolution is **lazy**: it runs the first time we contact a person (at
  invite-send), via a read-through cache — check the person's saved
  member id, and only call Unipile to resolve if it is missing, then
  persist it. The DM step and any other campaign reuse the cached id.
- The resolver classifies failures:
  - **Transient** (rate-limit, upstream 5xx, timeout) → leave the lead
    queued, **do not** consume the pacing allowance, **do not** write a
    failure event; it retries on the next tick.
  - **Permanent** (not found, deleted, private/unresolvable) → write a
    `failed` event to `linkedin_events` and move the `campaign_lead` to a
    terminal `failed` status so it is not retried forever.

### Acceptance detection (deep module: the accept matcher)

- Acceptance is detected **only** by the periodic check (~45 min); the
  `new_relation` notification path is not built.
- Each cycle, for each active/warming account, the product lists the
  account's **current connections** (their member ids) and the accept
  matcher returns which `awaiting_accept` leads — scoped to that account
  — have a cached member id now present in the connections set.
- The "mark accepted" query is rewritten to match a lead by **member
  id** (joining the lead's person to its `linkedin_member_id`
  identifier) instead of by invitation id, scoped to the lead's bound
  account, and gated on `status = 'awaiting_accept'` so repeated cycles
  are idempotent. On a match it flips the lead to `active`, stamps
  `accepted_at`, sets `current_step = 1`, and schedules the first DM
  immediately; it also writes an `accepted` event.
- The invitation id is still stored at send time and still used for the
  stale-invite withdrawal sweep; it is simply no longer the acceptance
  key.

### Reply detection and direction (deep module: the Unipile client)

- The message notification (webhook) stays as the prompt path, with the
  periodic chats check as backup, because message notifications arrive
  promptly (only acceptances carry the eight-hour lag).
- Message **direction** is derived from the real captured fields that
  identify the connected account versus the message sender — not from a
  presumed `is_sender` flag. A message from the connected account itself
  is ignored; a message from the prospect is a reply.
- On a prospect reply: the lead is matched by its stored chat id, flipped
  to `replied`, a `replied` event is recorded, and the **person** is
  suppressed across all campaigns (existing person-level suppression is
  unchanged). The self-halt bug is fixed entirely by correct direction in
  the client; the suppression module does not change.
- During the spike, confirm whether the chats-list response exposes
  direction; if it does not, the backup path derives direction the same
  way as the notification path.

### Connect / bind (modified: the webhook handler + client parsing)

- Webhook parsing is rebuilt to route on Unipile's **real event marker**
  in the body, replacing today's object-presence guesswork.
- Any inbound payload that carries our **decodable signed metadata**
  (the tenant+user token minted into the hosted-auth link) is treated as
  **connect-and-bind**: it upserts the `linkedin_account` and binds it to
  the tenant recovered from the metadata, regardless of which Unipile
  source the message arrives on. A payload **without** metadata that
  reports a status is treated as a **status update** to an existing
  account.
- Confirm against the captured connect payload that auto-bind works with
  no manual SQL, and that the signed metadata arrives within its
  validity window (currently fifteen minutes); extend the window only if
  the real echo timing requires it.

### Account status (modified: the webhook handler)

- Map Unipile's **real** status vocabulary (verified against captured
  payloads) to the existing `active` / `restricted` / `disconnected`
  account states; unknown statuses are ignored rather than clobbering the
  current value. Restriction/disconnect stops sending and records the
  provider reason; recovery re-activates.

### Webhook authentication (modified: the client + handler)

- The webhook is authenticated **only** by the fixed `Unipile-Auth`
  header, compared in constant time to the configured webhook secret.
  The endpoint returns service-unavailable if the secret is unset,
  unauthorized on a missing/incorrect header, and OK otherwise (including
  for event types we don't act on).
- The legacy fake-signature (HMAC body) acceptance path is **removed** —
  it only ever existed to satisfy self-authored tests, which now
  authenticate with the real header. The temporary capture log is
  removed once fixtures are frozen.

### Outbound client surface (deep module: the Unipile client)

- The client gains a **resolve** operation (profile URL + account →
  member id, or a classified error) and a **list-connections** operation
  (account → the member ids currently connected). Invite and message
  sends take a resolved member id rather than a URL. All Unipile
  knowledge — endpoints, payload shapes, status mapping, auth —
  stays behind the client's interface.

### Schema

- **No mandatory migration.** The member id reuses `person_identifiers`
  via a new identifier type value `linkedin_member_id`; the terminal
  `failed` lead state is a new value in the free-text `campaign_leads`
  status column.
- One **optional supporting index** may be added to keep the
  accept-match query fast (fetching `awaiting_accept` leads per account
  and joining their cached member id). Decide during implementation;
  it is not a correctness requirement at trial volume.

### API contracts

- **Inbound webhook:** accepts Unipile's real event payloads at the
  existing public webhook route, authenticated by the `Unipile-Auth`
  header equal to the configured webhook secret; single endpoint, routed
  in-handler on the real event marker.
- **Outbound:** resolve a profile to a member id with the account
  scoped; list an account's connections; send an invite addressed by
  member id; start a chat / send a message addressed by member id.

## Testing Decisions

A good test here exercises **external behavior against the real captured
payloads** and does not assert internal wiring. All four modules
confirmed for testing:

- **Payload parsing (client):** feed raw captured Unipile bytes into the
  parser and assert the typed result — acceptance vs. status vs. message
  vs. connect classification, the recovered ids, and the reply-direction
  decision in both directions. This is the heart of the "real shapes"
  fix. Prior art: the existing client unit tests with a fake HTTP doer —
  rebuilt on real fixtures.
- **Webhook → database lifecycle (handler):** POST the raw captured
  bodies with the real `Unipile-Auth` header and assert the resulting
  database state — connect auto-binds the account to the tenant, an
  acceptance flips the lead and schedules the first message, a prospect
  reply halts the lead and suppresses the person, our own message does
  **not** halt, and a restriction sets the account status. Prior art: the
  existing handler integration tests using an in-process HTTP server —
  rebuilt on real fixtures.
- **Member-id resolver:** drive a fake Unipile client plus the store and
  assert cache hit vs. miss (resolve only on miss, persist once) and the
  transient-vs-permanent failure handling (queued-and-retried vs.
  failed-and-terminal). Prior art: existing fake-client / repository
  tests.
- **Accept matcher:** feed a connections set and a set of
  `awaiting_accept` leads and assert exactly which leads flip, including
  the no-match and already-accepted (idempotent) cases. Pure logic, no
  external dependencies.

The tests that encode imagined payloads are **deleted** as part of this
work — they currently prove a contract that does not exist, and keeping
them would mask the real one.

## Out of Scope

- The `new_relation` acceptance **notification** path — deliberately not
  built; the periodic connections check is the sole acceptance path.
- **Inbound email / mailto unsubscribe** processing — a separate known
  operational gap, unchanged here.
- **Optimizing the connections list for very large accounts** — the
  check pulls the list as-is for now; recency/cursor optimization for
  accounts with thousands of connections is a later follow-up.
- **Pacing/warmup/breaker thresholds, the withdrawal horizon, and
  sourcing/search/ingest** — all unchanged.
- **On-demand verification of a live acceptance** — impractical given the
  eight-hour notification lag plus invite rate limits; validated via the
  connections list and frozen fixtures instead.

## Further Notes

- **Validation spike logistics:** the trial account and DSN already
  exist. Re-register the Unipile webhook sources against a fresh tunnel
  (quick-tunnel URLs are ephemeral and must be re-registered after each
  restart). Capture all the messages listed above, freeze the bytes as
  fixtures, then remove the temporary capture log.
- **Security:** the Unipile API key leaked into a transcript and must be
  rotated as part of this work.
- **Don't burn test recipients:** re-inviting the same LinkedIn profile
  within roughly a few weeks returns an "already invited recently" error
  that is recipient-specific — avoid withdrawing/re-inviting the same
  test profiles.
- **Idempotency is load-bearing:** the periodic check (and any webhook
  replays) must never double-act; every transition is gated on the
  current lead/account state.
- **Cleanup from the prior session:** a manually-inserted account row and
  a set of now-dead webhook registrations exist from the trial — verify
  and clean them up before live testing.
- This PRD supersedes the inbound assumptions baked into
  `docs/2026-06-09-linkedin-only-outreach`; the outbound rail, pacing,
  withdrawal, and metrics work from that effort stand.
