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

- [ ] New backend route `GET /api/v1/campaigns/<id>/metrics`
      returns:
  - `leads_total` (`campaign_leads` count for this campaign),
  - `sent_total` (`email_events` count where `event_type='sent'`,
    joined to `campaign_leads` of this campaign),
  - `sent_by_step` — array of `{step_order, count}`,
  - `replied_total` (`event_type='replied'`),
  - `bounced_total` (`event_type IN ('bounced','bounced-soft')`,
    de-duplicated per lead — a lead with 2 soft + 1 hard counts as
    1 bounce),
  - `unsubscribed_total` (`unsubscribes` rows for this tenant
    whose `email` matches a lead on this campaign).
- [ ] Frontend campaign-detail page at
      `frontend/src/app/(app)/campaigns/[id]/` renders these counts
      with a tidy header card per category, plus a per-step
      breakdown table.
- [ ] Near-real-time: page polls the metrics endpoint every 15s
      while open (no WebSocket needed). On poll, only the metric
      header card re-renders — the rest of the page is static.
- [ ] Unit tests for the metrics handler against real devpod
      Postgres: cover empty campaign, partial-progress campaign
      (some sent, some replied, some bounced), fully-completed
      campaign.
- [ ] Manual local verification: run a small campaign (5 leads),
      observe the dashboard update as the worker progresses through
      sends → first reply detected → first bounce.

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
