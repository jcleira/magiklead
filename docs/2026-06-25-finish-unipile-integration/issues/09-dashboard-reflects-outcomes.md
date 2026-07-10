# 09 — Dashboard reflects real inbound outcomes

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [07 — Accept matcher + connections check](./07-accept-matcher-connections-check.md), [08 — Reply detection + correct direction](./08-reply-detection-direction.md)

## What to build

With acceptance (#7) and replies (#8) now recorded as **real**
`linkedin_events`, verify the capacity/outcomes dashboard reflects
reality (sent, accepted, replied) and rebuild its metrics test on the
real event flow. The dashboard UI already exists from the prior effort
— this slice closes the loop so the numbers are **true**, since they
depend on the inbound events being recorded correctly.

## Acceptance criteria

- [x] The metrics/capacity endpoint shows accurate **sent / accepted /
  replied** counts driven by the real event flow (`invite_sent`,
  `accepted`, `dm_sent`, `replied`, `failed`).
- [x] End-to-end check: after an accept reconcile (#7) and a reply (#8),
  the dashboard counts reflect them.
- [x] `linkedin_metrics_test` is rebuilt to assert against the **real
  event flow** rather than imagined events.
- [x] The acceptance rate shown on the dashboard is consistent with what
  the pacer breaker reads (same `accepted` / `invite_sent` source).

## Modules touched

- Metrics / capacity handler (`backend/internal/handler/unipile.go` —
  `Capacity` + the metrics endpoint).
- Acceptance/event-count queries (`GetLinkedInAcceptanceStats` and the
  event-count queries in `backend/queries/linkedin_*.sql` + generated
  repository).
- Frontend dashboard (already built — verify the display).

## Test prior art

- `backend/internal/handler/linkedin_metrics_test.go`,
  `backend/internal/handler/linkedin_capacity_test.go` — **rebuild on
  the real event flow** produced by #7 and #8.

## Out of scope

- New dashboard UI — exists from the prior effort.
- Changing which metrics are shown.
