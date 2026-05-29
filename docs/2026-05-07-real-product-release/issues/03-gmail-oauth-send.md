# 03 — Gmail OAuth real send

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#2 — Suppression module + schema migrations](./02-suppression-module.md)

## What to build

Finish the `TODO` in `backend/internal/gmail/sender.go` so that the
worker actually delivers messages through the user's connected
Gmail account via the Gmail API (not SMTP, not MailHog). Capture
the returned `messageId` on the `email_events` row that records the
send — that `messageId` is what the reply-detection poller (#4) and
the bounce-detection poller (#5) match against to attribute
incoming messages back to specific sends.

The existing `gmail/service.go` + `gmail/oauth.go` already issue
the OAuth token dance; this slice extends the scope set to include
`gmail.send` (and prepares `gmail.readonly` for #4 to use later)
and wires `sender.go` to call `users.messages.send` with a
properly-formed RFC 5322 message.

After this slice, a campaign started via `POST /api/v1/campaigns/<id>/start`
that has a connected Gmail account sends step 1 to all active
leads from the user's real Gmail address, and every successful
send writes an `email_events` row with `event_type='sent'` and
`gmail_message_id=<returned id>`.

## Acceptance criteria

- [x] OAuth scope set includes `https://www.googleapis.com/auth/gmail.send`
      and `https://www.googleapis.com/auth/gmail.readonly` (the
      latter is consumed by #4; both granted in this slice so the
      user only sees one consent screen).
- [x] Existing OAuth flow in `internal/gmail/oauth.go` re-prompts
      users who connected before the scope change so the new scope
      is granted (token's `scope` claim checked at use time; missing
      `gmail.send` returns an actionable error and a re-connect
      link in the UI).
- [x] `internal/gmail/sender.go` `Send(...)` is implemented:
  - takes a `ConnectedAccount` (with refreshable token), a
    `Message{From, To, Subject, Body, Headers}`,
  - calls `users.messages.send` on the Gmail API,
  - returns the Gmail `messageId` and threadId,
  - propagates structured errors for: token expired, scope missing,
    message rejected (Gmail-side reasons), rate limited, network.
- [x] `internal/worker/sender.go` is updated to call
      `gmail.Sender.Send(...)` (gated by `suppression.IsSuppressed`
      from #2) and to write an `email_events` row with
      `event_type='sent'` + `gmail_message_id` populated.
- [x] Unit tests for `gmail.Sender` use the Google API client's
      mockable transport seam (or a `Doer`-style HTTP-client
      wrapper) and cover: happy path, token expired (auto-refresh
      then retry succeeds), scope missing (returns actionable
      error), 4xx message-rejected, 5xx rate limited, network error.
- [x] Unit tests for `worker/sender.go` cover: suppressed lead
      skipped (writes `skipped:suppressed`), happy send (writes
      `sent` + `gmail_message_id`), send failure (writes `failed`
      with classified reason).
- [ ] Manual local verification: connect operator's Gmail via the
      app, run a single-lead campaign to the operator's own
      secondary address, confirm message arrives in that inbox,
      confirm `email_events` row in DB has the returned
      `gmail_message_id`. **Deferred — blocked on the prerequisites
      below; tick once they're all green.**

## Manual-verification prerequisites

Surfaced during the 2026-05-15 attempt to exercise criterion #7
above. None are in this slice's scope, so this slice stops at the
unit + integration test layer.

- [ ] **Issue #8 ships the "Connect Gmail" UI** on the settings
      page. The `/gmail/auth-url` + `/gmail/callback` backend
      routes exist but nothing in the frontend triggers them, so
      the OAuth flow can't be exercised end-to-end through the
      product today.
- [ ] **OAuth client registered in Google Cloud Console** with the
      `gmail.send` + `gmail.readonly` scopes, the operator's Gmail
      added as a test user (consent screen still in "Testing"
      mode), and a public HTTPS redirect URI registered (Google
      rejects `.localhost`).
- [ ] **Public HTTPS endpoint for the callback** — a cloudflared
      quick-tunnel (or ngrok / Tailscale Funnel) in front of the
      api container, with `--http-host-header
      api-mvp.magiklead.localhost` so Traefik routes correctly.
      `GOOGLE_REDIRECT_URI` in `.env.backend` must match the
      tunnel URL exactly.
- [ ] **`GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` /
      `GOOGLE_REDIRECT_URI`** set in
      `~/.config/devpods/magiklead/.env.backend`; `devpods down &&
      devpods up` to pick them up.

## Modules touched

- `backend/internal/gmail/sender.go` (finishes the existing TODO).
- `backend/internal/gmail/oauth.go` (scope additions, scope
  validation at use time).
- `backend/internal/gmail/service.go` (any glue for the new client
  shape).
- `backend/internal/worker/sender.go` (replaces SMTP path, calls
  `gmail.Sender`, gates through `suppression.IsSuppressed`,
  records `gmail_message_id` on the event row).
- `backend/internal/repository/email_events.sql.go` (regenerated
  from the new `gmail_message_id` column added in #2).

## Test prior art

- `backend/internal/ingest/sources/edgar_test.go` and
  `wikidata_test.go` — `httptest.Server` + `Doer`-style HTTP
  stubbing pattern. Same shape applies for stubbing the Gmail API
  transport.
- `backend/internal/leads/smtp_verify.go` — existing email-sending
  module with a similar Sender shape; the Gmail variant follows
  the same surface contract.

## Out of scope

- Reply detection (`gmail.readonly` is *granted* here but consumed
  by [#4](./04-reply-detection.md)).
- Bounce classification — see [#5](./05-bounce-detection.md).
- The `List-Unsubscribe` and `List-Unsubscribe-Post` headers on
  outbound messages — see [#6](./06-unsubscribe.md).
- Outlook OAuth and custom SMTP — explicitly out of scope per PRD.
- Send rate limiting beyond Gmail's own quotas — out of scope per
  PRD.
- Per-step delay scheduling logic (it already exists in
  `worker/sender.go`); this slice only swaps the transport.
