# 02 — Suppression module + schema migrations

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#1 — Phase 0 unblock fixes](./01-phase-0-unblock.md)

## What to build

The foundational "do not email this person" plumbing that every
downstream send path will gate through. Per the PRD's Implementation
Decisions §Module shape, the `suppression` deep module exposes one
read (`IsSuppressed(tenant, email) → bool`) and a handful of write
functions for the four event types: reply, hard bounce, three
consecutive soft bounces, unsubscribe. The worker calls
`IsSuppressed` immediately before any send; the Gmail poller (issues
#4, #5) calls the event recorders post-classification.

Three schema migrations land in this slice — they are the columns
and table that the rest of Phase 1 reads and writes against. Each
must round-trip cleanly (up → exercise → down).

After this slice, the worker's send loop has an `IsSuppressed` gate
in front of every send (returns "skipped:suppressed" event rather
than calling the sender), and the four write functions exist with
unit-test coverage. No outbound behaviour changes yet because the
suppression list is empty — but the gate is wired and the table
shapes are stable.

## Acceptance criteria

- [ ] New migration adds `email_events.gmail_message_id TEXT` with
      an index. The column is nullable (existing rows have no value).
- [ ] New migration adds `gmail_accounts.last_history_id TEXT` and
      `gmail_accounts.last_polled_at TIMESTAMPTZ`. Both nullable.
- [ ] New migration creates table `unsubscribes`:
      `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`,
      `tenant_id UUID NULL REFERENCES tenants(id) ON DELETE CASCADE`
      (NULL = global), `email TEXT NOT NULL`, `reason TEXT NOT NULL`
      (manual / list-unsub / spam-complaint / reply / hard-bounce /
      soft-bounce-threshold), `created_at TIMESTAMPTZ NOT NULL
      DEFAULT now()`, unique on
      `(COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'), lower(email))`.
- [ ] All three migrations round-trip: up applies cleanly on a fresh
      DB, down removes the column/table cleanly. Verified by the
      `cmd/migrate` test path.
- [ ] New package `backend/internal/suppression/` with:
  - `IsSuppressed(ctx, tenantID, email) (bool, reason string, err)`.
    Returns true if there is a row in `unsubscribes` for either
    `(tenant_id, email)` or `(NULL, email)` (global), OR if the
    address has an `email_events` row classifying it as `bounced`
    (hard), OR three consecutive `soft-bounce` events.
  - `RecordReply(ctx, campaignLeadID, gmailMessageID) error`.
  - `RecordHardBounce(ctx, leadEmail, tenantID) error`.
  - `RecordSoftBounce(ctx, leadEmail, tenantID) error` — increments;
    on third consecutive, also writes to `unsubscribes` with
    `reason='soft-bounce-threshold'`.
  - `RecordUnsubscribe(ctx, email, tenantID *uuid.UUID, reason string) error`.
- [ ] `backend/internal/worker/sender.go` gates every send through
      `suppression.IsSuppressed`. On suppressed, write a `skipped`
      event with the reason and do not call the email sender.
- [ ] Unit tests in `backend/internal/suppression/suppression_test.go`
      cover every branch above against a real devpod Postgres,
      following the prior-art skip-pattern.
- [ ] `q.SearchPersons`-style queries in `backend/queries/` are
      extended for the new columns; sqlc regeneration produces
      compiling Go.

## Modules touched

- New deep module: `backend/internal/suppression/`.
- New migrations: `backend/internal/migrate/migrations/`.
- New queries: `backend/queries/suppression.sql` +
  `backend/queries/email_events.sql` (extension for
  `gmail_message_id`) + `backend/queries/gmail_accounts.sql`
  (extension for `last_history_id` / `last_polled_at`).
- Modified: `backend/internal/worker/sender.go` (adds the
  `IsSuppressed` gate).

## Test prior art

- `backend/internal/middleware/admin_test.go` — table-driven tests
  with manipulated context.
- `backend/internal/handler/privacy_test.go` — handler test pattern
  against real devpod Postgres.
- `backend/internal/ingest/resolver_test.go` — pattern for testing
  modules that read and write through the queries layer.

## Out of scope

- The Gmail History API poller itself — see [#4](./04-reply-detection.md)
  and [#5](./05-bounce-detection.md). This slice only adds the
  `email_events.gmail_message_id` column and the event-recorder API
  surface; the poller wires up later.
- The unsubscribe public endpoint — see [#6](./06-unsubscribe.md).
- The Gmail send path itself — see [#3](./03-gmail-oauth-send.md);
  this slice modifies `worker/sender.go` only to add the
  `IsSuppressed` gate around the existing call site.
