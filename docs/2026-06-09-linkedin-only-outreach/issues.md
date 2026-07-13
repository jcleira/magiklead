# Issues — 2026-06-09-linkedin-only-outreach

Source: [prd.md](./prd.md)

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Connect a LinkedIn account](./issues/01-connect-linkedin-account.md) | None |
| [x]  | 2 | [Source LinkedIn prospects](./issues/02-source-linkedin-prospects.md) | None |
| [x]  | 3 | [Author a LinkedIn campaign](./issues/03-author-linkedin-campaign.md) | [#2](./issues/02-source-linkedin-prospects.md) |
| [x]  | 4 | [Send the connection invite (paced)](./issues/04-send-connection-invite.md) | [#1](./issues/01-connect-linkedin-account.md), [#3](./issues/03-author-linkedin-campaign.md) |
| [x]  | 5 | [Acceptance → first DM](./issues/05-acceptance-first-dm.md) | [#4](./issues/04-send-connection-invite.md) |
| [x]  | 6 | [Follow-ups, reply-halt, reconcile poll](./issues/06-followups-reply-halt.md) | [#5](./issues/05-acceptance-first-dm.md) |
| [x]  | 7 | [Warmup ramp + acceptance breaker + withdrawal](./issues/07-pacer-warmup-breaker-withdrawal.md) | [#4](./issues/04-send-connection-invite.md) |
| [x]  | 8 | [Account restriction/disconnect + reconnect](./issues/08-account-restriction-reconnect.md) | [#1](./issues/01-connect-linkedin-account.md), [#4](./issues/04-send-connection-invite.md) |
| [x]  | 9 | [Metrics & capacity dashboard](./issues/09-metrics-capacity-dashboard.md) | [#4](./issues/04-send-connection-invite.md), [#5](./issues/05-acceptance-first-dm.md), [#6](./issues/06-followups-reply-halt.md) |

Dependency shape: **#1 and #2 are independent** (parallelizable). Spine: **#3 → #4 → #5 → #6**. #7 and #8 hang off the send path (#4). #9 needs the event-producing slices (#4/#5/#6).
