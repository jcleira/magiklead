# 09 — Per-campaign metrics dashboard

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#3 — Gmail OAuth real send](./03-gmail-oauth-send.md), [#4 — Reply detection via Gmail History API](./04-reply-detection.md), [#5 — Bounce detection](./05-bounce-detection.md), [#6 — Unsubscribe end-to-end](./06-unsubscribe.md)

## What to build

A campaign-detail dashboard that aggregates the four event types
recorded by sender (#3), reply poller (#4), bounce classifier (#5),
and unsubscribe endpoint (#6) into a single near-real-time view:
total leads, total sent (with per-step breakdown), total replied,
total bounced, total unsubscribed.

No open or click tracking — per PRD, Apple Mail Privacy Protection
has destroyed the open signal and click tracking is out of scope.

After this slice, the operator can tell at a glance whether a
campaign is producing replies or bouncing into a spam trap.

## Acceptance criteria

- [x] New backend route `GET /api/v1/campaigns/<id>/metrics`
      returns:
  - `leads_total` (`campaign_leads` count for this campaign),
  - `sent_total` (`email_events` count where `event_type='sent'`,
    joined to `campaign_leads` of this campaign),
  - `sent_by_step` — array of `{step_order, count}`,
  - `replied_total` (`event_type='replied'`),
  - `bounced_total` (`event_type IN ('bounced','soft-bounce')`,
    de-duplicated per lead — a lead with 2 soft + 1 hard counts as
    1 bounce). Event names match issue #2's vocabulary ('bounced'
    + 'soft-bounce'); the PRD draft text said 'bounced-soft' but
    the suppression module already established 'soft-bounce' and
    every event-writer keys off that string. Conceptual contract
    upheld.
  - `unsubscribed_total` (`unsubscribes` rows for this tenant —
    or global — whose `email` matches a campaign-lead on this
    campaign; lead-email resolved via canonical persons graph
    with legacy `leads` fallback).
- [x] Frontend campaign-detail page at
      `frontend/src/app/(app)/campaigns/[id]/` renders these counts
      with a tidy header card per category, plus a per-step
      breakdown table.
- [x] Near-real-time: page polls the metrics endpoint every 15s
      while open (no WebSocket needed). On poll, only the metric
      header card re-renders — the rest of the page is static.
      Implemented via a self-contained `MetricsSection` component
      with its own `useState` + `setInterval`; the page's existing
      leads / sequence / dialog blocks own no metrics state and so
      do not rerender on tick. Also pauses on `document.hidden`
      so a backgrounded tab doesn't keep polling.
- [x] Unit tests for the metrics handler against real devpod
      Postgres: cover empty campaign, partial-progress campaign
      (some sent, some replied, some bounced), fully-completed
      campaign. Implemented as build-tagged integration tests in
      `backend/internal/handler/campaigns_metrics_test.go`. Run
      with `devpods exec api go test -tags=integration
      ./internal/handler/...`. Tracer + tenant-scope (security
      invariant) + partial-progress (bounce dedup) +
      unsubscribe-match-by-email + fully-complete all pass.
- [ ]† Manual local verification: run a small campaign (5 leads),
      observe the dashboard update as the worker progresses through
      sends → first reply detected → first bounce. **Deferred —
      same prerequisite chain as #3/#4/#5/#6's manual steps
      (Connect Gmail UI from [#8](./08-settings-real-data.md), OAuth
      client in Google Cloud Console, public HTTPS callback tunnel,
      `GOOGLE_*` secrets in `.env.backend`). Plus a small operator
      fix: `UNSUBSCRIBE_SIGNING_SECRET` must be set in
      `~/.config/devpods/magiklead/.env.backend` so the api boots
      (added by #6; this devpod's env was never refreshed). Tick
      once those are all green.**

## Modules touched

- `backend/internal/handler/campaigns.go` — new `Metrics` method.
- `backend/queries/campaigns.sql` — new aggregation query (likely
  several `SELECT COUNT(...) WHERE ...` joined to `email_events`).
- `frontend/src/app/(app)/campaigns/[id]/page.tsx` — new metrics
  section; 15s polling.

## Test prior art

- `backend/internal/handler/tenant_leads_test.go` — handler test
  pattern.
- `backend/internal/handler/privacy_test.go` — validation paths.

## Out of scope

- Open / click tracking — explicitly out of scope per PRD.
- Cross-campaign aggregate dashboard (workspace-level stats) —
  defer; the v1 ask is per-campaign.
- Time-series charts (sends over time, reply rate by hour) —
  defer; counts only for v1.
- Per-lead drill-in to "all events on this lead" — exists
  elsewhere (campaign-leads view); not duplicated in metrics.
