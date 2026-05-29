# 04 — Reply detection via Gmail History API

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#3 — Gmail OAuth real send](./03-gmail-oauth-send.md)

## What to build

Per-mailbox polling of the Gmail History API on a two-minute
cadence. When a new inbound message is seen with an `In-Reply-To`
header matching a `gmail_message_id` we previously stored on an
`email_events` row, that's a reply to one of our sends: mark the
campaign-lead `replied`, halt all future sends to that lead, and
record the reply event through the suppression module (so the
"zero emails sent to replied leads" invariant from user story 27
is enforced by the same `IsSuppressed` gate that the worker
already checks before every send).

A "re-engage" button in the UI lets the user manually clear the
replied state for a lead — for the "they replied with an
out-of-office, not a real reply" case (user story 26).

After this slice, a real reply detected within two minutes halts
the sequence for that lead. The seven-day public launch invariant
(zero emails to replied leads) is now mechanically enforceable.

## Acceptance criteria

- [x] New deep module `backend/internal/gmail/poller/`:
  - `Tick(ctx, account ConnectedAccount) (newCursor string, events []ClassifiedEvent, err error)`.
  - Stubbable seam over the Gmail SDK so unit tests can run
    without hitting Google.
  - Classifies each new message: `reply` (matches a stored
    `gmail_message_id` via `In-Reply-To` or `References`),
    `bounce` (DSN format — classification logic lives here but
    handling lands in [#5](./05-bounce-detection.md)), or
    `unrelated` (ignored).
- [x] Worker tick that calls `gmail/poller.Tick` for every
      connected mailbox on a 2-minute cadence (alongside the
      existing sender tick — same worker process). Cursor stored on
      `gmail_accounts.last_history_id`, last-polled time on
      `last_polled_at`.
- [x] On a `reply` classification: call
      `suppression.RecordReply(...)`, update the relevant
      `campaign_leads.status='replied'`, write an `email_events` row
      with `event_type='replied'` + the inbound message's
      `gmail_message_id`.
- [x] Frontend: a "Re-engage" action on a replied lead's row in
      the campaign-leads view that clears the suppression-by-reply
      and flips `campaign_leads.status` back to `active`.
      Backend route `POST /api/v1/campaigns/<id>/leads/<lead-id>/reengage`.
- [x] Unit tests for `gmail/poller`:
  - happy path: cursor advances, reply classified, bounce
    classified, unrelated ignored,
  - edge cases: cursor missing (first poll), history-id rotated
    by Gmail (full mailbox re-sync triggered), expired token
    (auto-refresh then retry),
  - failure paths: network error, rate limit, malformed response.
- [x] Unit tests for worker tick: suppression event-recorder called
      on classified reply, campaign_lead.status updated, event row
      written.
- [x] Worker survives a transient Gmail outage: a failing tick logs
      the error and returns; next tick retries from the saved
      cursor.
- [ ] Manual local verification: connect operator's Gmail, run a
      campaign to a second mailbox, reply from the second mailbox,
      confirm within 2-3 minutes the campaign_lead is `replied` and
      no further sends fire. **Deferred — blocked on the same
      prerequisite chain as #3's manual step (Connect Gmail UI from
      [#8](./08-settings-real-data.md), OAuth client in Google Cloud
      Console, public HTTPS callback tunnel, `GOOGLE_*` secrets in
      `.env.backend`). Tick once those are all green.**

## Modules touched

- New deep module: `backend/internal/gmail/poller/`.
- New worker tick: extension of `backend/internal/worker/` to call
  the poller on each connected account.
- `backend/internal/suppression/` — called from poller after
  classification (already exposed by #2).
- Frontend: campaign-leads view in
  `frontend/src/app/(app)/campaigns/[id]/` — adds the re-engage
  action.
- Backend: `backend/internal/handler/campaigns.go` — re-engage
  route.

## Test prior art

- `backend/internal/ingest/sources/edgar_test.go` and
  `wikidata_test.go` — HTTP-mocked third-party API tests.
- `backend/internal/middleware/admin_test.go` — pattern for testing
  the worker tick with manipulated context and stub dependencies.
- `backend/internal/handler/privacy_test.go` — pattern for the
  re-engage handler test (validation paths only; behaviour
  exercised by Playwright in #14).

## Out of scope

- Bounce-event *handling* (suppression writes, sequence halt) —
  see [#5](./05-bounce-detection.md). This slice classifies bounces
  in the poller but routes them as no-ops until #5.
- Pub/Sub push notifications — explicitly out of scope per PRD;
  2-minute polling latency is accepted.
- Multi-recipient threading edge cases (a single inbound message
  matching multiple stored `gmail_message_id`s across different
  campaigns) — first-match wins for v1; the campaign-lead with the
  most recent send timestamp claims the reply.
