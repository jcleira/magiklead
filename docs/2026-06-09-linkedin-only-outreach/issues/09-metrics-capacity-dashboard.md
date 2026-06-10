# 09 — Metrics & capacity dashboard

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [04 — Send the connection invite (paced)](./04-send-connection-invite.md), [05 — Acceptance → first DM](./05-acceptance-first-dm.md), [06 — Follow-ups, reply-halt, reconcile poll](./06-followups-reply-halt.md)

## What to build

A tenant sees how their LinkedIn outreach is performing and how much
weekly capacity remains — invites sent, acceptance rate, DMs sent, reply
rate per campaign, plus remaining weekly invites on their connected
account.

End-to-end:
- Aggregate `linkedin_events` per campaign: invites sent, accepted
  (acceptance rate), DMs sent, replies (reply rate).
- Remaining weekly capacity from the pacer/account counters (this week's
  used vs the cap, accounting for warmup).
- A campaign metrics surface + a capacity indicator in the UI.

## Acceptance criteria

- [ ] A metrics endpoint returns per-campaign counts/rates derived from
  `linkedin_events`; assert the numbers against a seeded event set via
  DB + API.
- [ ] A capacity endpoint/value returns remaining invites this week for
  the connected account (cap − used, respecting warmup); assert against
  seeded counters.
- [ ] UI renders per-campaign acceptance rate + reply rate and the
  remaining-capacity indicator (Playwright happy path).

## Modules touched

- `internal/handler/...` metrics handler (mirror the existing
  per-campaign email metrics handler from the real-product-release).
- Aggregation queries over `linkedin_events`; a pacer/account capacity
  read.
- Frontend metrics dashboard + capacity indicator.

## Test prior art

- The existing per-campaign email metrics handler + its tests (the
  real-product-release shipped a campaign metrics dashboard — mirror
  it).

## Out of scope

- Cross-account aggregation (single account in MVP).
