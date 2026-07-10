# 01 — Validation spike: capture real Unipile payloads, freeze fixtures

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately. **This slice gates every
other slice in this effort.**

> **Partial progress (2026-07-01):** real payloads captured and documented
> in [../spike-captures.md](../spike-captures.md) — `account_status`
> (CREATION_SUCCESS), connections, sent-invitations, chats, and a message
> (with `is_sender` direction). Key finding: **auto-bind is broken** (no
> `notify_url`; connect webhook has no tenant metadata → deferred to #3).
> Still open: the `notify_url` connect payload, the `message_received`
> webhook shape, non-CREATION_SUCCESS status transitions, and the member-id
> resolve endpoint. Freezing into committed test fixtures happens in #2.
>
> **Progress (2026-07-06):** residue cleaned (manual `linkedin_accounts`
> row deleted; dead webhook registrations de-registered), fresh tunnel up
> with re-registered sources (delivery verified end-to-end), and the
> remaining read-only shapes captured: **resolve** endpoint (§6), an
> **outbound** `is_sender: 1` message (§5), the **account object** / own
> member id (§7), and the messaging-webhook **field list** (§8). All
> downstream `Confirmed from spike` fields filled. Key **not yet rotated**
> (verified still alive; dashboard-only). Still open: `message_received`
> raw bytes (needs one live inbound DM), the `notify_url` payload (#3),
> and non-CREATION_SUCCESS transitions (not emittable on demand —
> finding E).
>
> **Progress (2026-07-06, later):** capture rig re-verified end-to-end
> (tunnel alive, both registrations enabled at Unipile, probe POST
> reached the capture log; no `message_received` arrived yet). Old key
> re-confirmed alive. Everything automatable is done — the two
> remaining ACs are **human-gated** (Handoff posted): (1) rotate the
> key in the Unipile dashboard, then run the post-rotation verification
> recipe in spike-captures.md; (2) send one LinkedIn DM to the
> connected account ("Jose Corral") from any second profile while the
> rig is up, then have a session freeze the logged bytes into
> spike-captures.md.
>
> **Progress (2026-07-07):** the Unipile **trial expired**; jcleira is
> registering a **paid workspace** (new DSN + API key). The old
> workspace was emptied before abandonment (LinkedIn account
> disconnected — dashboard shows "Disconnected" — and both webhooks
> deleted; verified 0/0 via API), so the leaked key is defanged and
> dashboard rotation is moot. The 07-06 tunnel died with its process.
> The ordered re-setup sequence lives in spike-captures.md
> (§2026-07-07); captures §1–§8 stay valid across workspaces. Once the
> new creds land in `.env.backend`, a session runs the machine side
> (restart, verify, tunnel, register, probe), then the two human steps
> (connect LinkedIn, one inbound DM) close this slice.
>
> **Progress (2026-07-07, later):** plot twist ×2. (1) jcleira paid on
> the **existing** workspace (same DSN/key) — rotation is back on, now
> possible in the paid dashboard, and is the **only** open item.
> (2) The "missing" `message_received` raw bytes had **already been
> captured**: an organic cold-outreach DM hit the rig on 07-06 09:43,
> minutes before a host restart killed the containers + tunnel. Frozen
> in spike-captures **§9** (+ raw stash outside the repo); it exposed a
> real parser bug — webhook `is_sender` is bool, REST is int (finding
> F) — so AC-4 is now checked. Rig re-armed today (containers
> restarted, fresh tunnel, new registrations, probe verified). The
> LinkedIn account still needs re-connecting (operational, needed for
> #3+; not an AC of this slice).
>
> **Closed (2026-07-07):** key rotation **waived by jcleira** (exposure
> was session-context only; never repo/git) — with AC-4 done, that was
> the last open item. Same day: `devpods up` re-provisioned the stack
> and Traefik began redirecting http→https, which 308-broke the tunnel
> origin; rig rebuilt against `https://localhost:443`
> (`--no-tls-verify`) with fresh registrations and delivery re-verified
> end-to-end (recipe in spike-captures.md updated). All ACs resolved →
> row flipped in issues.md. #2 is unblocked.

## What to build

This is the precursor spike, not a feature slice. Run the integration
against the **live trial LinkedIn/Unipile account** over a fresh tunnel,
using the temporary capture log already present in the webhook handler,
and record the **real bytes** of every Unipile message the integration
depends on. Freeze those bytes as test fixtures. Rotate the leaked
Unipile API key. Clean up the residue left by the prior trial session.

No parser, handler, or test logic is built here — its only output is:
frozen fixtures + a rotated key + a clean slate + a set of confirmed
facts the downstream slices route on. **No assumed shapes** — if a
payload wasn't captured, it isn't a fixture.

## Acceptance criteria

- [ ] ~~Leaked Unipile API key **rotated** in the Unipile dashboard; the
  new value set in `~/.config/devpods/magiklead/.env.backend`
  (`UNIPILE_API_KEY`); the old key confirmed dead.~~ *(2026-07-06: old key
  verified still alive; confirmed absent from the repo tree + git
  history. Rotation is dashboard-only → human action, see Handoff.)*
  *(2026-07-07: trial expired → path changed from **rotate** to
  **replace** — jcleira registers a paid workspace (new DSN + key,
  never leaked). The old workspace was emptied first: LinkedIn account
  disconnected + both webhooks deleted; `GET /accounts` / `/webhooks`
  → 0 items, so the leaked key controls nothing while the trial
  endpoint lingers. Closes when the new creds are in `.env.backend`
  and verified — re-setup sequence in spike-captures.md.)*
  *(2026-07-07, later: jcleira paid on the **existing** workspace —
  DSN/key unchanged → the leaked key is live again, on a paid and
  functional dashboard. Back to the original path: rotate in the
  dashboard, update `.env.backend`, restart api+worker, verify old key
  401 / new key 200. This is the LAST open item on this slice.)*
  *(2026-07-07, final: **waived by jcleira** — no rotation. The
  exposure was session-context only, never repo/git; the key stays on
  the paid workspace. Decision recorded; nothing further to do.)*
- [x] Prior-session residue removed: the manually-inserted
  `linkedin_accounts` row is deleted, and the dead webhook
  registrations (from prior ephemeral tunnels) are de-registered at
  Unipile. *(2026-07-06: row `90dff2d4-…` deleted — table now empty;
  both stale registrations deleted, HTTP 200.)*
- [x] Webhook sources re-registered against a **fresh tunnel**
  (quick-tunnel URLs are ephemeral — must be re-registered after each
  restart). *(2026-07-06: `account_status` + `messaging` registered
  against the new tunnel; probe POST through the tunnel reached the
  capture log. Re-registration recipe recorded in spike-captures.md.)*
- [x] Captured and frozen as fixtures (raw bytes under a `testdata`
  dir): the **account-connected** message; **≥1 account-status**
  transition emitted during connect/sync; one **inbound** (prospect →
  account) message; one **outbound** (account → prospect) message; the
  **connections-list** response; the **sent-invitations-list** response;
  the **chats-list** response. *(Re-scoped 2026-07-01: freezing into
  committed `testdata` fixtures happens in #2; spike-captures.md is the
  capture of record. Captured: account_status/CREATION_SUCCESS,
  connections, sent-invitations, chats, inbound + outbound messages,
  resolve, account object. Still missing: `message_received` webhook
  raw bytes — needs one live inbound DM (see Handoff) — and further
  status transitions (not emittable on demand, finding E).)*
  *(2026-07-07: DONE — the `message_received` raw bytes arrived
  **organically** on 2026-07-06 09:43 (a real cold-outreach DM hit the
  live rig), frozen redacted in spike-captures **§9**, raw bytes
  stashed at `~/.config/devpods/magiklead/captures/`. Bonus: the real
  payload exposed a parser bug — webhook `is_sender` is **bool** vs int
  on REST (finding F) → #2/#8. Status transitions stay excused per
  finding E.)*
- [ ] ~~Confirmed: the account-connected payload **auto-binds** the
  account to the tenant with **zero manual SQL** (the signed metadata
  round-trips and is decodable on the inbound payload).~~ *(Moved to #3
  per 2026-07-01 finding A: auto-bind is broken — no `notify_url` in
  hosted-auth, no tenant metadata on the connect webhook. Cannot be
  confirmed until #3 adds `notify_url`.)*
- [ ] ~~Confirmed: the signed metadata arrives **within its validity
  window** (currently 15 min); note the real echo timing so #3 knows
  whether to extend it.~~ *(Moved to #3 with the above — not measurable
  until the metadata has a channel to echo back on.)*
- [x] Recorded for downstream slices (see Handoff): the real **event
  marker** field + values, the real **status vocabulary**, the fields
  that identify **message direction**, and the **resolve** /
  **connections-list** response shapes. *(2026-07-06: all `Confirmed
  from spike` fields in #2/#3/#4/#5/#7/#8 filled from
  spike-captures.md.)*

## Modules touched

- The **temporary capture log** in the webhook handler
  (`backend/internal/handler/unipile.go` — the `UNIPILE-CAPTURE` log
  block). Leave it in place for this slice; #2 removes it.
- `backend/cmd/linkedin-demo/` — prospect/spike tooling for seeding test
  recipients.
- Unipile dashboard (webhook source registration + API key rotation).
- Devpod env (`~/.config/devpods/magiklead/.env.backend`).
- A new `testdata` fixtures directory for the frozen raw bytes.

## Test prior art

N/A — this slice produces fixtures, it does not add tests. The frozen
bytes are consumed by `backend/internal/handler/unipile*_test.go` and
`backend/internal/linkedin/unipile/*_test.go` in later slices.

## Out of scope

- Removing the capture log — **#2** does that, once the fixtures are
  consumed (US 30).
- The `new_relation` acceptance notification — deliberately **not**
  captured (8-hour lag + invite rate limits make on-demand capture
  impractical) and **not** required; acceptance is detected by the
  connections check (#7).
- Any parser/handler/resolver logic — those are #2–#8.
- Re-inviting the same test profiles to force an acceptance — LinkedIn
  returns a recipient-specific "already invited recently" error; don't
  withdraw/re-invite the same profiles.

## Handoff

When the ACs above all pass, BEFORE flipping this slice's row in
`issues.md`, post the following block to the user verbatim:

- **URL / artefact to visit**:
  - The app's **LinkedIn connect screen** (`GET /api/v1/linkedin/auth-url`
    → hosted-auth wizard) — a human completes the connect to produce the
    real `account.connected` payload.
  - The **Unipile dashboard** — for API-key rotation and webhook-source
    registration against the fresh tunnel.
  - The frozen **`testdata` fixtures directory** — for sign-off that the
    captured bytes are complete and correct.
- **Action required**:
  1. Connect a LinkedIn account through the app so the real
     `account.connected` payload is captured.
  2. Rotate the leaked Unipile API key and update `.env.backend`.
  3. Confirm auto-bind worked with **zero manual SQL**.
  4. Sign off that all listed payloads were captured and frozen.
- **Where to record the decision**:
  - In `issues.md` (a one-line note next to this slice's row): the
    frozen fixtures dir path + the confirmed event marker / status
    vocabulary / direction fields + "auto-bind: yes/no".
  - In the downstream slice files, under each `Confirmed from spike`
    field:
    - **#2** — the real event marker + auth header scheme.
    - **#3** — auto-bind-works-with-zero-SQL + metadata echo timing.
    - **#4** — the real status vocabulary.
    - **#5** — the resolve-response shape / endpoint.
    - **#7** — the connections-list shape.
    - **#8** — the direction-identifying fields + whether the chats-list
      exposes direction.

Slices #2–#8 MUST NOT start until the `Confirmed from spike` field each
one depends on is filled.
