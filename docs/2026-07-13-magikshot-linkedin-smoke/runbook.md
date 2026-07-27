# MagikShot × LinkedIn — Smoke Runbook

The single document from which any future session runs and judges the
LinkedIn smoke. It assumes only this file and the repo. Everything here
is a real probe — a SQL query against a real table, an api log pattern,
an HTTP route, or a `systemctl`/`devpods` command — not a vibe. Symbols
were verified against the code on 2026-07-25; where a probe names a
constant, table, column, event string, or route, it exists in the tree.

**Human-only actions** (the founder, in a browser): sign-up/onboarding,
the hosted-auth connect click, the three approval gates (§3), and the
manual warm-up activity (§7). **Everything else is curl / SQL / log**
and runs AFK.

## Pointers

| Thing | Value |
|-------|-------|
| Smoke pod | `smoke` (worktree on the `magiklead-smoke` branch) |
| Local api | `https://api-smoke.magiklead.localhost` |
| Local app | `https://smoke.magiklead.localhost` |
| Public api (webhooks) | `https://magiklead-smoke.magikshot.com` |
| Tunnel service | `systemctl --user status magiklead-smoke-tunnel.service` |
| Nightly dump | `systemctl --user list-timers magiklead-smoke-dump.timer` (03:17, `Persistent=true`) |
| Dumps (14-day retention) | `~/.local/share/magiklead-smoke/dumps/` |
| Base-URL override | `MAGIKLEAD_PUBLIC_API_URL` in `<smoke-worktree>/.env` (gitignored) |
| Slate verifier | `docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh` |
| Webhook CLI | `devpods exec api go run ./cmd/unipile-webhooks <list\|register\|prune>` |
| Standup / re-provision | `docs/2026-07-13-magikshot-linkedin-smoke/standup.md` |
| Fixture loader (test data) | `devpods exec api go run ./cmd/seed` |

Sibling issues: pod + tunnel + dumps (#01), base-URL override (#02),
webhook CLI (#03), Unipile hygiene / slate (#04), this book (#05), the
connect ceremony (#06), and the run to verdict (#12).

---

## 0. Invariants — read before touching anything

**Never `devpods seed` and never `devpods down` the `smoke` pod for the
duration of the smoke.** Weeks of real campaign state — sent invites,
acceptances, the reply that decides the whole exercise — live only in
this pod's Postgres. `seed` truncates and replants; `down` destroys the
volumes. Either one silently ends the smoke. The only safety net is the
nightly `pg_dump` (03:17) at `~/.local/share/magiklead-smoke/dumps/`;
treat it as disaster recovery, not a licence to reset. `cmd/seed` and
`devpods seed` are for the **`mvp`** pod, never here.

**All `devpods …` commands in this book target the pod of the worktree
you run them from — run them from the smoke worktree** so they hit the
`smoke` pod, not `mvp`.

**Auth for probes.** `GET /health` and `POST /api/v1/webhooks/unipile`
are public. Every other route under `/api/v1` needs a Clerk bearer.
Prefer the no-auth path for daily checks: `devpods psql` (SQL) and
`devpods logs api` (logs) read the same tables and emit the same lines
the API does. When you must call an authed route, mint a token with
`tests/e2e/helpers/clerk.ts` (see `docs/2026-07-09-devpod-https-auth-handoff.md`)
or read it from the founder's browser session on the settings page.

---

## 1. Day-0 checklist

Run top to bottom. Each line has its own verification; do not proceed on
assumption.

1. **Pod healthy, never-seed acknowledged (#01).**
   `devpods status` → all services healthy; `devpods psql -c '\l'` shows
   the pod's own database. Say out loud: *no seed, no down on smoke.*
2. **Tunnel up + publicly reachable (#01).**
   `systemctl --user status magiklead-smoke-tunnel.service` → `active
   (running)`. Then from the public internet:
   ```
   curl -si https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile
   ```
   Expect **405** (GET not allowed) — an api-generated response proves
   the tunnel reaches the pod's api. Confirm the request line lands in
   `devpods logs api`.
3. **Base-URL override in effect (#02).**
   ```
   devpods exec api printenv APP_URL
   ```
   Must print `https://magiklead-smoke.magikshot.com` (not the
   `.localhost` literal). This is what the backend stamps into the
   hosted-auth `notify_url`, so a wrong value here means the connect
   webhook never arrives.
4. **Key active + clean account slate (#04).**
   ```
   bash docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh
   ```
   Expect all PASS: Unipile key works (`GET /accounts` → 200), slate
   empty (`total_count 0`), both pods `/health` 200, and webhook
   wrong-auth → **401** (proves `UNIPILE_WEBHOOK_SECRET` is loaded — a
   401, not a 503). Key rotation was **waived 2026-07-25** (founder:
   keep the existing key); only the clean-slate half is load-bearing.
5. **Webhook registrations live (#03).**
   ```
   devpods exec api go run ./cmd/unipile-webhooks list
   ```
   Expect the three sources the rail listens on, all with
   `REQUEST_URL = https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile`
   and `ENABLED = true`:
   - `account_status` — hosted-auth connect completion,
   - `users` / `new_relation` — a prospect accepted,
   - `messaging` / `message_received` — a reply landed.
   If any are missing or point at an old hostname, re-run:
   ```
   devpods exec api go run ./cmd/unipile-webhooks register \
     --base-url https://magiklead-smoke.magikshot.com
   ```
   (idempotent — creates only what's missing). `--dry-run` first if
   unsure.
6. **Founder onboarded in the smoke pod.** Founder signs up at
   `https://smoke.magiklead.localhost` (Clerk dev keys) and completes
   onboarding. Prereqs from `CLAUDE.md` apply: the `magiklead-backend`
   JWT template (`{"email": …}`) must exist, and the founder's email
   must be in `ADMIN_EMAILS` if admin surfaces are needed. Verify the
   tenant exists: `devpods psql -c 'SELECT id, created_at FROM tenants ORDER BY created_at;'`

---

## 2. The connect ceremony (executed in #06)

The one-time bind of the founder's warmed LinkedIn account to the smoke
tenant, through the tunnel, with **zero manual SQL**.

**Precondition:** the account has cleared the §7 warm-up completion
criterion. Do not connect a cold shell.

Steps (founder's browser on the laptop):

1. Founder opens `https://smoke.magiklead.localhost/settings` → **Connect
   LinkedIn**. The backend mints a hosted-auth link (`GET
   /api/v1/linkedin/auth-url`) carrying a signed metadata token that
   binds the tenant.
2. Founder completes Unipile's hosted-auth wizard (LinkedIn credentials
   + any checkpoint) in the redirected page.
3. On success the browser returns to `…/settings?linkedin=connected`
   (failure → `…/settings?linkedin=error`). **Important:** the redirect
   alone does *not* create the local row — that happens only when the
   webhook fires.

What to watch (no SQL needed to confirm the happy path):

- `devpods logs api` shows a `POST /api/v1/webhooks/unipile` request line
  returning **200** — the `account.connected` delivery.
- The handler calls `UpsertLinkedInAccount`, so a `linkedin_accounts`
  row appears bound to the tenant, provenance = the webhook, **no manual
  INSERT**:
  ```
  devpods psql -c "SELECT unipile_account_id, status, created_at FROM linkedin_accounts;"
  ```
  Row starts `status='connecting'` and settles to `active`.
- The settings UI polls `GET /api/v1/linkedin/accounts` and flips to
  *connected* on its own.

**If the row never appears** (webhook unreachable): confirm the tunnel
service is running and `unipile-webhooks list` shows the registration at
the current hostname; re-`register` if the hostname rotated. Last-resort
per project memory — insert the `linkedin_accounts` bind row by hand —
but that **fails pass criterion #1**, so exhaust the webhook path first.

---

## 3. Approval gates

Three founder gates. None is skippable; each is recorded so a later
session can see the go was given.

| Gate | When | Who | Recorded where |
|------|------|-----|----------------|
| **List** (#09) | Before any prospect is loaded | Founder | The curated CSV committed under `docs/2026-07-13-magikshot-linkedin-smoke/` + the loader's planted/updated/skipped report pasted into #09 |
| **Copy** (#10) | Before the campaign exists | Founder | The approved sequence copy file committed under the docs folder; #10's row records approval |
| **Campaign start** (#11) | Before the pacer sends the first invite | Founder | #11 records the explicit start + the §7 warm-up completion check that gated it |

The rule the gates encode: nothing goes onto LinkedIn — no data load, no
copy, no send — without the founder saying yes in writing first.

---

## 4. Daily observation routine

Concrete probes, run once a day. All are `devpods psql` / `devpods logs`
(no auth). "Smoke tenant" below means the founder's tenant id from step
1.6; LinkedIn leads are `campaign_leads` rows with `linkedin_account_id
IS NOT NULL`.

### 4a. Sends vs. allowance

Invites actually sent, per UTC day:
```sql
SELECT date_trunc('day', created_at) AS day, count(*) AS invites
FROM linkedin_events
WHERE event_type = 'invite_sent'
GROUP BY 1 ORDER BY 1;
```
Account counters + warm-up anchor:
```sql
SELECT unipile_account_id, status, warmup_started_at,
       daily_invite_count, daily_reset_at,
       weekly_invite_count, weekly_window_started_at,
       acceptance_rate, last_error
FROM linkedin_accounts;
```
Compare against the pacer's policy — constants in
`backend/internal/linkedin/pacer/pacer.go`:

| Warm-up week (since `warmup_started_at`) | Daily cap | Effective weekly cap |
|---|---|---|
| 1 | 8 | 56 |
| 2 | 12 | 84 |
| 3 | 16 | 100 |
| 4+ | 20 | 100 |

Derivation: `WarmupWeek1DailyCap=8`, `+warmupStepPerWeek=4`/week, capped
at `DefaultDailyCap=20`; weekly = `min(StandardWeeklyCap=100,
7×daily)`. **No day's `invites` may exceed that week's daily cap**, and
`weekly_invite_count` within the current window may not exceed the
effective weekly cap.

### 4b. Acceptance breaker inputs (trailing 7 days)

```sql
SELECT
  count(*) FILTER (WHERE event_type='invite_sent') AS invites_7d,
  count(*) FILTER (WHERE event_type='accepted')    AS accepted_7d
FROM linkedin_events
WHERE created_at > now() - interval '7 days';
```
The breaker (`BreakerThreshold=0.20`, `BreakerMinSample=20`) pauses the
account — `Allowance` returns 0 — when `accepted_7d / invites_7d < 0.20`
**and** `invites_7d ≥ 20`. Below 20 invites it abstains (a young account
that simply hasn't been accepted yet is never paused). A paused account
is the protection working (§6), not a bug.

### 4c. Lead-state counts + event timeline

```sql
SELECT status, count(*)
FROM campaign_leads
WHERE linkedin_account_id IS NOT NULL
GROUP BY status ORDER BY 2 DESC;
```
Expected statuses: `awaiting_accept` (invited, waiting), `active` (accepted,
DM sequence running), `exhausted` (sequence finished), `replied`,
`not_accepted` (invite withdrawn after 21d), `suppressed` (person
suppressed).
```sql
SELECT created_at, event_type, step, campaign_lead_id
FROM linkedin_events
ORDER BY created_at DESC
LIMIT 50;
```
Event vocabulary written by the engine: `invite_sent`, `accepted`,
`dm_sent`, `withdrawn`, `skipped`, `failed`.

### 4d. Worker loop cadences (expect in `devpods logs worker`)

- **Send / DM loop — every 60s.** "LinkedIn DM loop: N leads due…".
  Sends invites within allowance and DMs to accepted leads.
- **Reconcile loop — every 45m.** "LinkedIn reconcile loop started
  (checking every 45m)". The authoritative accept/reply backstop: it
  intersects each account's current connections with its
  `awaiting_accept` leads and applies any reply the webhook missed.
  LinkedIn's `new_relation` notification can lag ~8h — reconcile is what
  guarantees eventual detection.
- **Withdrawal loop — hourly.** "LinkedIn withdrawal loop started
  (checking hourly)". Withdraws invites unaccepted past the **21-day**
  horizon (≤50/tick): `CancelInvitation` at Unipile → `withdrawn` event
  → lead `not_accepted`.

### 4e. UI spots (founder's browser, authed)

- Capacity widget — `GET /api/v1/linkedin/capacity` (weekly remaining vs
  effective cap).
- Account status — `GET /api/v1/linkedin/accounts` (connected / status).
- Campaign metrics — `GET /api/v1/campaigns/{id}/linkedin-metrics`
  (funnel: `invites_sent`, `accepted`, `dms_sent`, `replies` + rates).

### 4f. Webhook liveness

```
systemctl --user status magiklead-smoke-tunnel.service     # active (running)
```
Confirm a recent inbound `POST /api/v1/webhooks/unipile` in `devpods
logs api`. **Recovery** if silent: re-run `unipile-webhooks list`, and
`register --base-url https://magiklead-smoke.magikshot.com` if the
hostname rotated.

---

## 5. Seven pass criteria (the success definition)

Each maps to a concrete verification. The smoke passes only if all seven
hold **and** the win condition is met.

1. **Webhook-bound connect, zero manual SQL.**
   `devpods logs api` shows the `account.connected` `POST
   /api/v1/webhooks/unipile` → 200, and the `linkedin_accounts` row's
   `created_at` matches that moment. Provenance is the webhook →
   `UpsertLinkedInAccount`; no hand-written INSERT in the session log.

2. **Loaded prospects searchable.**
   The curated people (loaded in #08/#09) carry LinkedIn identifiers:
   ```sql
   SELECT count(*) FROM person_identifiers WHERE identifier_type = 'linkedin_url';
   ```
   and the LinkedIn-channel lead search returns them with a
   `linkedin_url` attached — `POST /api/v1/leads/search` (authed);
   `const IdentifierType = "linkedin_url"` is the key it matches on.

3. **Campaign created from approved copy.**
   The campaign's stored sequence equals the approved copy file (#10):
   ```sql
   SELECT linkedin_sequence FROM campaigns WHERE id = '<campaign>';
   ```
   Diff the `linkedin_sequence` JSONB steps against the committed
   approved-copy file — they must match verbatim.

4. **Pacing held.**
   The §4a per-day query shows no day exceeding that week's daily cap,
   and `weekly_invite_count` within the effective weekly cap. No manual
   override of the pacer anywhere in the window.

5. **Acceptance advances to DM automatically.**
   For an accepted lead the timeline shows an `accepted` event (from the
   `new_relation` webhook or the 45m reconcile) **followed by** a
   `dm_sent` for the same `campaign_lead_id`, with the lead flipping
   `awaiting_accept → active` and `accepted_at` set — no human step
   between accept and first DM:
   ```sql
   SELECT created_at, event_type, step
   FROM linkedin_events
   WHERE campaign_lead_id = '<lead>'
   ORDER BY created_at;
   ```

6. **Reply halts and suppresses.**
   A prospect reply flips the lead to `replied` and writes a
   **person-keyed** suppression row, and **no** automated message goes
   out to that person afterward across any campaign:
   ```sql
   -- person-keyed reply suppression (reason 'reply')
   SELECT id, tenant_id, person_id, reason, created_at
   FROM unsubscribes
   WHERE person_id IS NOT NULL AND reason = 'reply';

   -- across-campaign halt: after the reply, that person's other leads
   -- show 'skipped' + status 'suppressed', never a later 'dm_sent'
   SELECT cl.id, cl.campaign_id, cl.status, e.event_type, e.created_at
   FROM campaign_leads cl
   JOIN linkedin_events e ON e.campaign_lead_id = cl.id
   WHERE cl.person_id = (
     SELECT person_id FROM unsubscribes
     WHERE reason='reply' AND person_id IS NOT NULL LIMIT 1)
   ORDER BY e.created_at;
   ```
   Pass = zero `dm_sent` with `created_at` after the reply's timestamp;
   later attempts are `skipped`.

7. **Clean disconnect.**
   `DELETE /api/v1/linkedin/accounts/{id}` returns **204**; Unipile's
   account list is then empty and the local row is gone:
   ```
   bash docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh   # total_count 0
   devpods psql -c "SELECT count(*) FROM linkedin_accounts;"       # 0
   ```
   The handler calls `svc.Disconnect` at Unipile and deletes the row; a
   repeat DELETE is still 204 (idempotent).

**Win condition — the point of the whole exercise:** ≥ 1 genuine
prospect reply recorded in-app. Verify: a `campaign_leads` row at
`status='replied'`, and `replies ≥ 1` from `GET
/api/v1/campaigns/{id}/linkedin-metrics` (the LinkedIn funnel:
`invites_sent` / `accepted` / `dms_sent` / `replies`, each derived from
`linkedin_events`).

---

## 6. Expected non-failures

These look like problems and are not. Do not "fix" them.

- **~8h acceptance lag.** LinkedIn's `new_relation` notification is slow.
  The 45m reconcile loop is the backstop — an acceptance always lands
  within a reconcile cycle even if the webhook is late. Nothing to do.
- **Laptop downtime pauses sending.** The pod and tunnel live on the
  laptop; asleep = no sends and no inbound webhooks. On wake, the pacer
  resumes and reconcile back-fills any accept/reply missed. No data is
  lost — just delayed.
- **The acceptance breaker pausing a fresh account.** On a small sample
  a low early accept-rate can trip the breaker (§4b). That is the
  circuit protecting the account from a restriction, not a defect. Let
  it recover on its own.
- **Never withdraw invites by hand.** Re-inviting soon after a manual
  withdrawal burns the recipient for weeks (spike learning). The hourly
  withdrawal loop owns the 21-day horizon; leave it alone.

---

## 7. Warm-up protocol

A brand-new LinkedIn account that starts automating is the fastest way to
a restriction. Before the account is connected (§2) and before any
campaign starts (#11 go-gate), the founder warms it **manually** for
2–3 weeks.

**Account creation.** The founder's real identity — real name, a real
photo (generate a professional headshot with magikshot), a complete
headline/About/experience. Not a persona, not a stock photo.

**Manual activity cadence (2–3 weeks, no automation of any kind):**
- Fill the profile to LinkedIn "All-Star" completeness in week 1.
- Sign in and browse normally most days — read the feed, react to a few
  posts, view profiles. Human rhythm, not a script.
- Send a handful of organic connection requests to genuinely known
  contacts, spread across the window. Build a real, accepting network.
- Zero third-party automation touches the account during warm-up. The
  pod's pacer stays off this account until §2 connect + #11 start.

**Completion criterion (all must hold — #11's go-gate checks this):**
- [ ] **Age:** ≥ 14 calendar days since the account was created.
- [ ] **Profile:** photo, headline, About, ≥ 1 experience entry, and
      location all set (profile strength "All-Star" / complete).
- [ ] **Organic network:** ≥ 20 connection requests sent to real known
      contacts, with ≥ 10 accepted.
- [ ] **Activity:** signed in with normal browsing on ≥ 10 of the
      trailing 14 days.
- [ ] **Clean standing:** zero restriction, checkpoint, or
      verification warnings in the account's LinkedIn notifications or
      email over the entire window.

If every box is checked, warm-up is complete and #11 may start the
campaign. If any box is unchecked, **hold** — do not connect for
sending or start the campaign — and extend the warm-up until it is.

---

## Appendix — probe quick reference

```bash
# --- health / reachability ---
curl -si https://magiklead-smoke.magikshot.com/api/v1/webhooks/unipile   # 405 (tunnel→api)
devpods exec api printenv APP_URL                                        # public base URL
bash docs/2026-07-13-magikshot-linkedin-smoke/verify-slate.sh            # key + slate + gates

# --- webhooks ---
devpods exec api go run ./cmd/unipile-webhooks list
devpods exec api go run ./cmd/unipile-webhooks register --base-url https://magiklead-smoke.magikshot.com

# --- ops ---
systemctl --user status magiklead-smoke-tunnel.service
systemctl --user list-timers magiklead-smoke-dump.timer
ls -lh ~/.local/share/magiklead-smoke/dumps/
```

```sql
-- invites/day vs cap
SELECT date_trunc('day', created_at) AS day, count(*)
FROM linkedin_events WHERE event_type='invite_sent' GROUP BY 1 ORDER BY 1;
-- account counters
SELECT unipile_account_id, status, warmup_started_at, daily_invite_count,
       weekly_invite_count, acceptance_rate, last_error FROM linkedin_accounts;
-- breaker inputs (7d)
SELECT count(*) FILTER (WHERE event_type='invite_sent') AS invites_7d,
       count(*) FILTER (WHERE event_type='accepted')    AS accepted_7d
FROM linkedin_events WHERE created_at > now() - interval '7 days';
-- lead states
SELECT status, count(*) FROM campaign_leads
WHERE linkedin_account_id IS NOT NULL GROUP BY status;
-- recent events
SELECT created_at, event_type, step, campaign_lead_id
FROM linkedin_events ORDER BY created_at DESC LIMIT 50;
-- reply suppression
SELECT person_id, reason, created_at FROM unsubscribes
WHERE person_id IS NOT NULL AND reason='reply';
```
