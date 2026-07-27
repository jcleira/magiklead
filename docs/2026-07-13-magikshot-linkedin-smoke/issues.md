# Issues — 2026-07-13-magikshot-linkedin-smoke

Source: [prd.md](./prd.md)

Slices 1–5 and 7 are grabbable immediately. The calendar-critical
path is 1→6 (pod + tunnel + day-one connect ceremony, which starts
the 2–3 week warm-up clock); the code slices (7, 8) land during the
warm-up window. Founder gates: key rotation (4), account creation +
connect (6), list approval (9), copy approval (10), campaign go (11).

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Smoke devpod standup: worktree, named tunnel service, nightly dumps](./issues/01-smoke-devpod-standup.md) | None |
| [x]  | 2 | [Parameterize the pod's public base URL](./issues/02-parameterize-public-base-url.md) | None |
| [x]  | 3 | [Unipile webhook registration CLI: list / register / prune](./issues/03-unipile-webhook-registration-cli.md) | None |
| [x]  | 4 | [Unipile hygiene: rotate API key, verify clean account slate](./issues/04-unipile-hygiene.md) — rotation **waived** 2026-07-25 (founder: keep existing key); slate cleaned (personal acct disconnected 2026-07-24, `total_count 0`) | None |
| [x]  | 5 | [Smoke runbook + warm-up protocol + seven pass criteria](./issues/05-smoke-runbook-warmup-protocol.md) | None |
| [ ]  | 6 | [Day-one connect ceremony: new account, hosted-auth bind through the tunnel](./issues/06-day-one-connect-ceremony.md) | [#1](./issues/01-smoke-devpod-standup.md), [#2](./issues/02-parameterize-public-base-url.md), [#3](./issues/03-unipile-webhook-registration-cli.md), [#4](./issues/04-unipile-hygiene.md), [#5](./issues/05-smoke-runbook-warmup-protocol.md) |
| [x]  | 7 | [LinkedIn identifier unification: one type, one canonical URL form](./issues/07-linkedin-identifier-unification.md) | None |
| [ ]  | 8 | [Curated prospect CSV loader with planted/updated/skipped report](./issues/08-curated-prospect-csv-loader.md) | [#7](./issues/07-linkedin-identifier-unification.md) |
| [ ]  | 9 | [Curate, founder-approve, and load the 30–50 prospect list](./issues/09-curate-approve-load-list.md) | [#1](./issues/01-smoke-devpod-standup.md), [#8](./issues/08-curated-prospect-csv-loader.md) |
| [ ]  | 10 | [Plays from magikshot.com analysis + hand-written, approved copy](./issues/10-plays-and-approved-copy.md) | [#1](./issues/01-smoke-devpod-standup.md) |
| [ ]  | 11 | [Campaign creation + explicit manual start under the pacer](./issues/11-campaign-creation-manual-start.md) | [#6](./issues/06-day-one-connect-ceremony.md), [#9](./issues/09-curate-approve-load-list.md), [#10](./issues/10-plays-and-approved-copy.md) |
| [ ]  | 12 | [Run to verdict: daily observation, pass criteria, ≥1 reply, clean disconnect](./issues/12-run-to-verdict-clean-disconnect.md) | [#11](./issues/11-campaign-creation-manual-start.md) |
