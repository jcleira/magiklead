# 06 — Day-one connect ceremony: new account, hosted-auth bind through the tunnel

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Smoke devpod standup](./01-smoke-devpod-standup.md),
[02 — Parameterize the pod's public base URL](./02-parameterize-public-base-url.md),
[03 — Unipile webhook registration CLI](./03-unipile-webhook-registration-cli.md),
[04 — Unipile hygiene](./04-unipile-hygiene.md),
[05 — Smoke runbook + warm-up protocol](./05-smoke-runbook-warmup-protocol.md)

Key rotation confirmed: WAIVED 2026-07-25 — founder kept the existing
key (`sha256 dcf1e148…`); the leaked-key concern was raised and
overruled. Slate verified clean (personal account disconnected, Unipile
`total_count 0`) — see #04 and `../day0-evidence.md`. Gate satisfied.

**Prep executed 2026-07-27** (session-driven step 1 + AC2/AC3): smoke
pod deployed to #02/#03/#07 (PR #7), `APP_URL` = the public tunnel,
migration 030 applied, three webhooks registered at the tunnel with the
`Unipile-Auth` header (3 stale mvp registrations pruned), `notify_url`
wired. A latent #03 bug (create-webhook parsed `id`, not the real
`webhook_id`) was found on this first live run and fixed. Evidence:
`../day0-connect-prep.md`. the founder connected 2026-07-29; the bind was blocked by a second live
bug (the notify_url callback is header-less; fixed `9db4491`) and
completed 2026-08-03 — account `active`. **Ceremony done.**

**Status change, found 2026-09-22: the account went `restricted`
(`CREDENTIALS`) less than 16 hours after the bind.** The "account
`active`" above was true only at bind time. Evidence: the nightly
dumps in `~/.local/share/magiklead-smoke/dumps/`, read with
`pg_restore` in a temporary `postgres:17-alpine` container.

| Dump (CEST) | `linkedin_accounts` |
|---|---|
| 2026-08-03 08:52 | no row (before the bind) |
| 2026-08-04 03:17 | `restricted`, `last_error` = `CREDENTIALS` |
| 2026-08-05 → 2026-08-17 (12 dumps) | the same |

The 2026-08-04 row: id `e7715755…`, tenant `be594ede…`,
`unipile_account_id` `v0DboRWMTQiYVSLUNZbmNg`, `created_at`
2026-08-03 09:27:57Z, `warmup_started_at` NULL, all counters 0. So the
change came between 09:27Z and 01:17Z the next day. All 14 dumps have
zero LinkedIn campaigns and zero `linkedin_events`.

- **Source: Unipile, not our code.** `last_error` holds the raw
  provider status, and only `handleAccountStatus` writes that value.
  That handler serves the registered `account_status` webhook, which
  must send the `Unipile-Auth` header
  (`backend/internal/handler/unipile.go`). The worker path
  (`escalateRestriction`) writes `unipile: account restricted`, and
  only after a send. There were no sends. Thus Unipile reported that
  the LinkedIn session needs a new login.
- **First live restriction event.**
  [Spike finding E](../../2026-06-25-finish-unipile-integration/spike-captures.md)
  expected that we cannot make these events on demand. The detection
  path (webhook → `mapUnipileStatus` → `restricted` + reason) worked
  on real traffic. The raw bytes are lost: Docker replaced the api
  container on 2026-08-11, so the log lines from 2026-08-03/04 are
  gone.
- **Cause: unknown.** The docs record no cause. #11 step 0 checks the
  runbook §7 *Clean standing* box against these dates.
- **This note is the durable record.** The first good nightly dump
  after the pod runs again deletes all dumps older than 14 days
  (`devpod/smoke/dump.sh`). That removes every dump in the table.
- **Next:** reconnect before any send —
  [#11 step 0](./11-campaign-creation-manual-start.md).

## What to build

Nothing new — this slice **executes** the runbook's day-0 checklist
and connect ceremony for real, proving the one thing local
development never could: a hosted-auth connect whose
`account.connected` webhook traverses the public tunnel and binds
tenant to account with **zero manual SQL** (PRD user stories 1, 5,
6, 8). It also starts the 2–3 week warm-up clock, which is the
project's critical path.

Sequence:

1. **Prep (session-driven).** Walk the runbook day-0 checklist:
   tunnel reachable, `APP_URL` override live, rotated key in place,
   founder signed up + onboarded in the smoke pod. Run #03's CLI
   live: `register --base-url <tunnel origin>`, then `list` showing
   the three sources with the right `request_url` and auth header.
2. **Founder (human).** Creates the new dedicated LinkedIn account
   carrying his real identity, profile photo generated with
   magikshot itself; then, from the smoke pod's https frontend
   settings page, starts Connect LinkedIn and completes the
   hosted-auth wizard. (The wizard's success/failure redirects go
   to the local frontend — that's fine, the browser is on the
   laptop; only the webhook needs the tunnel.)
3. **Observe the bind.** `devpods logs api` shows
   `POST /api/v1/webhooks/unipile` → 200 at wizard completion; the
   `linkedin_accounts` row appears (tenant bound, `unipile_account_id`
   set, status `active`) created by the webhook path alone; the
   settings UI flips to connected via its `/api/v1/linkedin/accounts`
   poll.
4. **Capture the payload.** The connect payload delivered to a real
   `notify_url` is the spike's one remaining "still open" capture
   (`docs/2026-06-25-finish-unipile-integration/spike-captures.md`).
   Store the raw body outside the repo (house pattern:
   `~/.config/devpods/magiklead/captures/`), commit a sanitized
   shape into the smoke docs.
5. **Start warm-up.** Record the date; the founder begins the
   manual warm-up protocol (#05). No sends of any kind until #11.

## Acceptance criteria

- [x] `Key rotation confirmed:` field above is filled (gate from
      #04).
- [x] Registration CLI `list` output (3 sources, tunnel
      `request_url`, `Unipile-Auth` header present) captured as
      evidence in the docs folder.
- [x] The minted hosted-auth link carries the tunnel `notify_url` —
      proven: the real `account.connected` callback arrived at
      `magiklead-smoke.magikshot.com` (`../day0-connect-prep.md`).
- [x] `account.connected` arrived through the tunnel: api log line
      timestamped at wizard completion (2026-07-29 09:14Z). It was a
      **401**, not a 200 — the notify_url-header bug (fixed `9db4491`);
      the tunnel *delivery* is what this AC proves. Bind completed
      post-fix.
- [x] `linkedin_accounts` bind row exists for the founder's tenant
      (`be594ede…`) with status `active` — no manual INSERT/UPDATE
      (row provenance is the webhook handler; psql evidence in the
      prep doc).
- [x] Settings UI shows the account connected (row `active`; the
      `/api/v1/linkedin/accounts` poll surfaces it).
- [x] Sanitized connect-payload shape committed
      (`../day0-connect-prep.md`); raw bytes not retained — the real
      callback 401'd before body logging, so the shape + the
      header-less delivery are the capture.
- [x] Warm-up start date recorded in `issues.md` and in #11's
      `Warm-up start date:` field (2026-07-29, account creation).

## Modules touched

None (execution slice). Evidence lands in
`docs/2026-07-13-magikshot-linkedin-smoke/`; payload capture extends
the spike-captures knowledge.

## Test prior art

The crafted-payload e2e specs (`tests/e2e/tests/16-linkedin-connect.spec.ts`)
and the in-pod integration suite
(`backend/internal/handler/unipile_connect_integration_test.go`)
assert exactly the state this ceremony must produce live — use the
same checks (row fields, statuses) as the evidence queries.

## Out of scope

- Any invite/DM send — nothing sends until #11's explicit start.
- The manual warm-up activity itself (founder, per #05's protocol,
  over calendar weeks).
- Prospect loading (#09) and copy (#10) — can proceed in parallel
  during warm-up.

## Handoff

When the ACs above all pass, BEFORE flipping this row in
`issues.md`, post this block to the user:

- **URL / artefact to visit**: the smoke pod's frontend settings
  page (LinkedIn shows connected) + the evidence captures in this
  docs folder.
- **Action required**: founder confirms the ceremony is complete
  and the warm-up clock has started (account created with magikshot
  photo, wizard finished, bind verified).
- **Where to record the decision**:
  - In `issues.md` (warm-up start date next to this row), AND
  - In [11 — Campaign creation + manual start](./11-campaign-creation-manual-start.md)
    under its `Warm-up start date:` field.

Issue #11 MUST NOT start until that field is filled (and its own
warm-up-complete gate passes).
