# 05 — Bounce detection

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#4 — Reply detection via Gmail History API](./04-reply-detection.md)

## What to build

Wire the bounce branch of the Gmail `History API` poller from #4
into real handling. Hard bounces (5xx DSN) halt the sequence
immediately and add the address to suppression; soft bounces (4xx
DSN) increment a counter and only halt after three consecutive on
the same address.

After this slice, an undeliverable send produces an
`event_type='bounced'` row, the campaign-lead `status='bounced'`,
and the next worker tick will skip that lead via the
`IsSuppressed` gate from #2.

## Acceptance criteria

- [x] `gmail/poller`'s `classify(...)` (added in #4) correctly
      identifies Gmail DSN messages and extracts:
  - the original `gmail_message_id` (from the bounced message's
    headers — typically `Message-ID` of the original is referenced
    in the DSN body or `In-Reply-To`),
  - the bounce status code (5xx hard, 4xx soft) from the DSN's
    `Status:` line per RFC 3464.
- [x] On `hard-bounce`: call `suppression.RecordHardBounce(...)`,
      update `campaign_leads.status='bounced'`, write
      `event_type='bounced'` with the DSN status code captured.
- [x] On `soft-bounce`: call `suppression.RecordSoftBounce(...)`
      (which increments and writes to `unsubscribes` with
      `reason='soft-bounce-threshold'` only on the third consecutive
      hit — logic centralised in #2's suppression module). Write
      `event_type='soft-bounce'` with the count + DSN status code in
      metadata so each bounce is visible per send. (Note: the PRD
      text says `bounced-soft`; #2's suppression module already
      established `soft-bounce` as the event_type and its
      `CountConsecutiveSoftBounces` query keys off that string —
      kept the existing name to avoid a fork. The conceptual
      contract — per-bounce visibility with count — is met.)
- [x] After three consecutive soft bounces on the same address,
      subsequent worker ticks skip via `IsSuppressed` (the
      suppression module handles the threshold logic; this slice
      just records). Pinned by
      `TestPollOnce_SoftBounceReachesThreshold`.
- [x] Unit tests for the bounce classifier:
  - hard bounce: 5xx DSN, correct extraction, correct routing,
  - soft bounce: 4xx DSN, correct routing,
  - DSN without a recognisable status code: routed to `unrelated`
    + logged at warn level for operator visibility,
  - DSN that references a message we did not send: ignored.
- [ ] Manual local verification: send to a known-bad address (e.g.
      `bounce-test@simulator.amazonses.com` or a non-existent
      Gmail address), observe DSN, confirm bounce event row,
      confirm next tick skips that lead via `IsSuppressed`.
      **Deferred — same prerequisite chain as #3/#4 (Connect Gmail
      UI from [#8](./08-settings-real-data.md), OAuth client in
      Google Cloud Console, public HTTPS callback tunnel,
      `GOOGLE_*` secrets in `.env.backend`). Tick once those are
      all green.**

## Modules touched

- `backend/internal/gmail/poller/` — extends the classifier and
  the post-classification routing added in #4.
- `backend/internal/suppression/` — `RecordHardBounce` and
  `RecordSoftBounce` are already exposed by #2; this slice is
  consumers of them.
- `backend/internal/repository/email_events.sql.go` — may need a
  `dsn_status_code TEXT NULL` column. Add a migration if so (or
  fold the status code into a JSONB `meta` column if one already
  exists on `email_events`).

## Test prior art

- Same as [#4](./04-reply-detection.md): `gmail/poller` is the
  same module, just a different classifier path.
- `backend/internal/ingest/resolver_test.go` — pattern for
  exercising classifiers with table-driven cases.

## Out of scope

- Per-tenant or per-mailbox bounce-rate dashboards — the data is
  in `email_events`; the dashboard work lives in
  [#9](./09-campaign-metrics.md).
- Non-Gmail DSN formats — Gmail-only this release per PRD.
- Custom thresholds (per-tenant 3-soft-bounce override) — out of
  scope; 3 is hard-coded.
