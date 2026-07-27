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
- [ ] Registration CLI `list` output (3 sources, tunnel
      `request_url`, `Unipile-Auth` header present) captured as
      evidence in the docs folder.
- [ ] The minted hosted-auth link carries the tunnel `notify_url`
      (log or captured request).
- [ ] `account.connected` arrived through the tunnel: api log line
      with a 200, timestamped at wizard completion.
- [ ] `linkedin_accounts` bind row exists for the founder's tenant
      with status `active` — and no manual INSERT/UPDATE happened
      (psql evidence; the row's provenance is the webhook).
- [ ] Settings UI shows the account connected.
- [ ] Sanitized connect-payload shape committed; raw stored outside
      the repo.
- [ ] Warm-up start date recorded in `issues.md` and in #11's
      `Warm-up start date:` field.

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
