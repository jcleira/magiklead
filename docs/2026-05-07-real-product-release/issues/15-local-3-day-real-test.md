# 15 — Local 3-day real test

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [#14 — Playwright E2E suite](./14-playwright-e2e-suite.md)

## What to build

Phase 5 of the PRD — a three-day endurance run on the local
devpod, real Gmail send, real PDL lookups, real prospects, real
replies, real bounces. The only thing not "real production" is the
URL: still `mvp.magiklead.localhost`. The campaign is for
magikshot (the operator's own product), ~100 prospects sourced via
PDL.

Any P1 bug pauses the three-day clock and restarts the window.
"P1" per PRD: any email sent to a replied lead, send failure rate
above 10%, settings page showing wrong data, sequence not pausing
on reply, bounce or unsubscribe silently dropped, privacy erasure
broken, site unreachable, worker-tick Sentry panic.

After this slice, the operator has lived in their own product for
72 hours under real conditions and has either a clean window or a
list of P1s that get fixed and re-clocked.

## Acceptance criteria

- [ ] Campaign created: real magikshot ICP, ~100 prospects from
      PDL, multi-step sequence (3–5 steps with realistic delays).
- [ ] Campaign run lasts 72 consecutive hours from start. Worker
      sender + poller running the whole time. Operator checks at
      least 3× per day for issues.
- [ ] Outcomes recorded in this file's "Outcome" section:
  - prospects emailed (target: ~100),
  - sends attempted vs. successful (failure rate < 10%),
  - replies detected, sequence halted within 2 minutes each time,
  - bounces detected, classified correctly,
  - unsubscribes (if any), processed correctly,
  - **zero** sends to replied leads (binary, hard-blocking).
- [ ] Any P1 bug pauses the clock, gets a fix commit, and resets
      the 72-hour window. Each pause is logged in this file with
      time-stamp + description.
- [ ] No Sentry panics during the window (note: Sentry isn't wired
      yet locally; for this phase, operator monitors api + worker
      logs directly for panics).
- [ ] Operator records final sign-off: "3-day clean window
      complete on YYYY-MM-DD, ready for production wiring."

## Modules touched

- None (verification + endurance run, not new code).
- Bug fixes during the run touch whichever module is broken;
  small commits onto the magiklead-mvp branch.

## Test prior art

- This is a new endurance-style test. The Playwright suite
  ([#14](./14-playwright-e2e-suite.md)) covers feature regression;
  this slice covers real-conditions regression that automation
  cannot see.

## Out of scope

- Production deploy — that's Phase 6 ([#16](./16-hetzner-vps-and-api.md)
  onwards).
- Public sign-ups — out of scope until Phase 8
  ([#26](./26-seven-day-public-launch.md)). This phase has no
  external users.
- magiktext campaign — runs concurrently in Phase 8 only;
  Phase 5 is magikshot-only to keep the operator's load
  manageable.
- A formal P0/P1/P2 triage process beyond what the PRD specifies.

## Blocking bugs found — 2026-05-28 pre-flight

This slice cannot start: the **real Gmail OAuth connect flow is
non-functional**. #3 and #8 are marked `[x]†` but their AFK walks
inserted `gmail_accounts` rows directly via SQL, so the actual
OAuth callback path was never exercised. Four confirmed defects
(verified against the live devpod), all on the connect path the
operator hits in step 1:

1. **Callback is behind `ClerkAuth`.** `GET /api/v1/gmail/callback`
   returns **401** with no bearer (confirmed via curl). Google's
   OAuth redirect arrives with `?code=…&state=…` and no
   Authorization header, so the round-trip can never complete.
   The route (`cmd/api/main.go:197`) sits inside the
   `r.Use(middleware.ClerkAuth)` group (`:157`). Fix: move the
   callback to a public route and bind the tenant/user via a
   signed `state` param (today `state` is random — see
   `handler/gmail.go:42` `randomState()`).
2. **Redirect URI mismatch.** `.env.backend` has
   `GOOGLE_REDIRECT_URI=…/api/gmail/callback` (no `/v1`); that
   path **404s**. The real route is `/api/v1/gmail/callback`. Fix
   the env var (and register the exact value in Google Cloud
   Console).
3. **`userID := tenantID // placeholder`** (`handler/gmail.go:73`).
   `SaveAccount` is called with the tenant UUID as the user id;
   FK-violates `gmail_accounts.user_id → users.id`. Fix: carry the
   real Clerk user id through `state` and resolve it in the
   callback.
4. **Connected email is faked.** Callback reads
   `email := r.URL.Query().Get("email")` and defaults to
   `"connected@gmail.com"` (`handler/gmail.go:66-68`) — the real
   mailbox address is never fetched from Google despite the
   granted `userinfo.email` scope. Fix: call Google's userinfo
   endpoint with the access token and store the real address;
   otherwise every campaign sends `From: connected@gmail.com`.

These are code fixes in `handler/gmail.go` + `cmd/api/main.go` +
`gmail/service.go` + `.env.backend` — i.e. **#3 / #8 territory,
not #15** (#15 is "Modules touched: None"). They must land (with
tests, ideally a real-callback integration test that the SQL-stub
walks skipped) before this slice's Phase A completes.

## Setup runbook — argos 2026-05-28

Ordered. Phases A–D. Nothing here is done yet; this is the
operator's turnkey path once the blocking bugs are fixed.

### Phase A — fix the OAuth connect path (code; blocking)

Fix bugs 1–4 above. Acceptance: a real Google account completes
the consent screen and lands one `gmail_accounts` row with the
real refresh token + real email, `user_id` = the connecting
Clerk user. Argos can do this AFK with a real-callback test — say
the word and it's the next slice of work (it's #3/#8, so it ships
as its own small commit, not under #15).

### Phase B — provision real third-party creds (operator-only)

**B1 — Google Cloud OAuth app:**
1. console.cloud.google.com → new project (or reuse) → APIs &
   Services → Enable the **Gmail API**.
2. OAuth consent screen → External → add your Workspace address as
   a test user (or publish). Scopes: `gmail.send`,
   `gmail.readonly`, `userinfo.email` (these match
   `gmail/oauth.go:23-27`).
3. Credentials → Create OAuth client ID → Web application.
   Authorized redirect URI **must exactly equal**
   `http://api-mvp.magiklead.localhost/api/v1/gmail/callback`
   (the corrected value from bug #2).
4. Copy the client ID + secret.

**B2 — PDL:** peopledatalabs.com → sign up → the $98/mo plan
(5k credits covers ~100 prospects with multiples of headroom) →
copy the API key.

**B3 — load into the devpod:** put these in
`~/.config/devpods/magiklead/.env.backend`:
```
GOOGLE_CLIENT_ID=<from B1>
GOOGLE_CLIENT_SECRET=<from B1>
GOOGLE_REDIRECT_URI=http://api-mvp.magiklead.localhost/api/v1/gmail/callback
PDL_API_KEY=<from B2>
```
Then `devpods up` (reloads api + worker; verified idempotent).
Confirm: `devpods exec api env | grep -E 'PDL|GOOGLE'` shows real
values and api `/health` is 200.

### Phase C — connect mailbox + build the campaign

1. **Connect Gmail:** sign in at `mvp.magiklead.localhost` →
   `/settings` → "Connect Gmail" (`settings/page.tsx:73` →
   `/api/v1/gmail/auth-url`) → complete Google consent. Verify:
   `devpods exec postgres psql -U devpod -d devpod -tAc "SELECT
   email, refresh_token NOT LIKE 'stub%' AS real FROM
   gmail_accounts;"` shows your real address + `t`.
2. **Onboard magikshot:** `POST /api/v1/websites/analyze
   {"url":"https://magikshot.com"}` → `POST /api/v1/plays/generate`
   → pick the ICP play that fits.
3. **Source ~100 prospects (real PDL):** `POST /api/v1/leads/search`
   with the magikshot ICP filters (titles + industries +
   company_size + locations) and `with_email:true`,
   `limit:100`. Confirm `pdl_called:true` in the response (proves
   the real PDL path fired, not canonical-only).
4. **Save + campaign:** `POST /api/v1/tenant_leads` for each
   person_id (quota counts unique persons), `POST /api/v1/campaigns`,
   `POST /api/v1/sequences/generate {play_id,campaign_id}` for a
   3–5 step sequence. To tune step delays/copy: no PATCH route
   exists yet (#13 fix-in-flight #2) — edit via
   `UPDATE campaigns SET sequence = jsonb_set(...)` until that
   route lands.
5. **Link + assign mailbox:** `POST /api/v1/campaigns/{id}/leads
   {person_ids:[…]}`, set `campaigns.gmail_account_id` to the
   connected account.

### Phase D — start + monitor 72h

1. `POST /api/v1/campaigns/{id}/start`. Clock starts.
2. **P1 watchdog** — the headline invariant ("zero sends to
   replied leads") as a single query; **any row returned = P1**:
   ```sql
   SELECT cl.id
   FROM campaign_leads cl
   WHERE EXISTS (SELECT 1 FROM email_events r
     WHERE r.campaign_lead_id=cl.id AND r.event_type='replied')
     AND EXISTS (SELECT 1 FROM email_events s
       WHERE s.campaign_lead_id=cl.id AND s.event_type='sent'
       AND s.created_at > (SELECT MIN(r2.created_at)
         FROM email_events r2
         WHERE r2.campaign_lead_id=cl.id AND r2.event_type='replied'));
   ```
   Other P1 checks: failure rate
   `sent_failed/(sent_ok+sent_failed) > 0.10`; bounce → lead
   `status='bounced'` + event row; unsub → `unsubscribes` row;
   `/health` != 200; `devpods logs worker | grep -i panic`.
   Argos can wire this as a `/loop` or scheduled agent that polls
   every ~5 min and appends violations here with timestamps —
   that replaces the "operator checks 3× per day" criterion with
   an actual watchdog.
3. **Outcomes** recorded in the section below as the window runs.
4. **P1 → pause:** log timestamp + description here, land a fix
   commit, reset the 72h clock.

## Outcome

_(empty — the 72-hour window has not run. Blocked on Phase A
+ Phase B above.)_
