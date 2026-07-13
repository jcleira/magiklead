# HANDOFF — Finish the LinkedIn (Unipile) integration

**Goal:** make the LinkedIn-only outreach feature actually work end-to-end
against **real Unipile**, not just against self-authored tests.

**Status in one line:** the **outbound** half works for real; the
**inbound** half (detecting accepts and replies) is broken because the
code was written against *assumed* Unipile payloads that don't match what
Unipile actually sends. This was discovered by wiring a live Unipile trial
account and exercising it.

> The existing automated tests are green, but they are **not trustworthy**:
> they construct webhook payloads that match our (wrong) parser, so they
> prove the wrong contract. Fixing the tests to use **real Unipile payload
> shapes** (documented below) is part of the work.

---

## Definition of done

Full lifecycle verified against real Unipile, with tests built on real
payload fixtures:

connect account → send paced invite → **detect acceptance** → auto first
DM → follow-ups → **detect reply → halt + suppress** → account
restriction/disconnect handling → metrics.

---

## ✅ Verified working this session (do NOT redo)

1. **Connecting a LinkedIn account at Unipile** — a real account
   ("Jose Corral", LINKEDIN, status OK) is connected under the trial DSN
   `https://api47.unipile.com:17756`. Live `GET /api/v1/accounts` returns it.
2. **Sending a connection invite** — a real invite was delivered to a real
   profile via `POST /api/v1/users/invite` (account_id + resolved
   provider_id + message). Confirmed received on LinkedIn.
3. **Backend builds; unit + integration tests pass.**
4. **Webhook auth gate** — see Gap G1: the original HMAC check rejected all
   real Unipile webhooks (401). A fix is implemented (uncommitted) and
   proven live (401 → 200).

---

## 🔧 Gaps to fix (the actual work)

Each gap: symptom → root cause → evidence → files → fix.

### G1 — Webhook auth (IMPLEMENTED, uncommitted; needs cleanup)
- **Symptom:** every real Unipile webhook returned **401**; nothing inbound
  could ever bind/transition.
- **Root cause:** handler required `X-Unipile-Signature` = HMAC-SHA256(body,
  secret). **Unipile does not HMAC-sign webhook bodies** — its create-webhook
  API has no signing field; the only auth is a **static custom header** you
  configure (`Unipile-Auth: <secret>`), echoed verbatim on every call.
- **Evidence:** create-webhook schema (only `headers` array, no secret);
  webhooks overview doc; proven live (Unipile-style request → 401 before,
  200 after).
- **Files:** `internal/linkedin/unipile/unipile.go` (added `VerifyAuthToken`),
  `internal/handler/unipile.go` (`Webhook()` now accepts `Unipile-Auth` OR
  the legacy HMAC for tests), `internal/handler/unipile_test.go` (fake).
- **Remaining:** there is a **TEMP `UNIPILE-CAPTURE` log line** in
  `Webhook()` — **remove it** before committing. Then commit the auth fix
  with a test that uses the real account_status payload (G6 shape).

### G2 — Acceptance detection is fundamentally mis-keyed (HIGH)
- **Symptom:** an accepted invite never flips the lead to active; no auto DM.
- **Root cause #1 (webhook):** the `users` webhook `new_relation` event
  **does not contain `invitation_id`**. Our handler
  (`handleInvitationAccepted` → `MarkLinkedInAccepted`) matches by
  `campaign_leads.linkedin_invitation_id`, which is never present in the
  event. Must correlate by the new contact's **`user_provider_id`** instead.
- **Root cause #2 (timing):** `new_relation` is **polled by Unipile, delayed
  up to 8 hours** (LinkedIn has no real-time accept event). So the webhook
  cannot be the timely path — the **reconcile poll must be the primary
  mechanism**.
- **Root cause #3 (reconcile):** `Module.AccountActivity` lists
  `/users/invite/sent` and keeps items with `status == "accepted"`, keyed by
  `invitation_id`. Per Unipile, accepted invites **leave the pending list**
  rather than flip to an "accepted" status, and correlation is by provider_id
  not invitation_id. So the reconcile path is also likely a no-op.
- **Evidence:** `docs.unipile.com/docs/detecting-accepted-invitations` —
  `new_relation` payload = `{event, account_id, user_provider_id,
  user_public_identifier, user_profile_url, user_picture_url}`, no
  invitation_id; "up to 8 hours"; recommends diffing the **relations list**.
- **Files:** `internal/linkedin/unipile/unipile.go` (`webhookPayload`,
  `ParseWebhook`, `AccountActivity`), `internal/handler/unipile.go`
  (`handleInvitationAccepted`), `backend/queries/campaign_leads.sql`
  (`MarkLinkedInAccepted`), repository, and the invite-send path that must
  **persist the recipient `provider_id` on the campaign_lead at send time**.
- **Fix:**
  1. At invite time, store the recipient's resolved `provider_id` on the
     campaign_lead (new column or reuse). Keep `linkedin_invitation_id` for
     withdrawal, but acceptance correlates on provider_id.
  2. Parse `new_relation` → `{account_id, user_provider_id}`; flip the lead
     whose stored provider_id matches.
  3. Make the **reconcile poll list current relations/connections** and flip
     any awaiting_accept lead whose provider_id now appears as a connection.
     This is the path that actually fires within hours.

### G3 — Reply / message-direction is wrong (HIGH)
- **Symptom:** the sequence would halt on our **own** outgoing DMs (treats
  them as prospect replies) and suppress the person.
- **Root cause:** `ParseWebhook` reads `is_sender` (a `*int`) to set
  `FromSelf`. **Unipile's new-message payload has no `is_sender`.** Direction
  is determined by comparing `account_info.user_id` (the connected account)
  with `sender.attendee_provider_id`.
- **Evidence:** `docs.unipile.com/docs/new-messages-webhook`. Payload
  top-level: `account_id, account_type, account_info, event, chat_id,
  timestamp, webhook_name, message_id, message, sender, attendees,
  attachments`. Unipile: *"compare `account_info.user_id` with
  `sender.attendee_provider_id` to know if the linked account is the sender."*
- **Files:** `internal/linkedin/unipile/unipile.go` (`webhookPayload`,
  `ParseWebhook`); tests.
- **Fix:** read `account_info.user_id` + `sender.attendee_provider_id`; set
  `FromSelf = (account_info.user_id == sender.attendee_provider_id)`. Keep
  using `chat_id`/`message_id` (those names are correct).

### G4 — Outbound recipient id is a URL, Unipile needs a member id (MED)
- **Symptom:** worker-sent invites/DMs will fail (422 / wrong recipient).
- **Root cause:** the worker passes `lead.LinkedinUrl` (from
  `person_identifiers.identifier_type='linkedin_url'`) straight to Unipile as
  `provider_id`. Unipile needs the **resolved member id** (e.g.
  `ACoAA...`), obtained via `GET /api/v1/users/{public_id|url}?account_id=…`.
  (This session's manual invite only worked because the profile was resolved
  first.)
- **Files:** `internal/worker/linkedin.go` (`Recipient: lead.LinkedinUrl` in
  the invite path and DM path), `internal/linkedin/unipile/unipile.go`
  (`SendInvitation`/`SendMessage`).
- **Fix:** resolve the profile → `provider_id` before sending, or store the
  resolved `provider_id` in `person_identifiers` at ingest time and send that.

### G5 — `account.connected` auto-bind is unverified (MED)
- **Symptom:** connecting through the app may not bind the account to the
  tenant (this session bound it manually via SQL).
- **Root cause (suspected):** Unipile's connect-success likely arrives on the
  `account_status` source. Our `ParseWebhook` routes anything containing an
  `AccountStatus` object to `EventAccountStatus` (a status *update* on an
  existing row), not `EventAccountConnected` (which *inserts* + binds the
  tenant from the signed metadata). So a fresh connect may never create the
  row. **Capture the real connect payload and confirm.**
- **Files:** `internal/linkedin/unipile/unipile.go` (`ParseWebhook`),
  `internal/handler/unipile.go` (`handleAccountConnected` /
  `handleAccountStatus`).

### G6 — Tests encode imagined payloads (HIGH, cross-cutting)
- Rewrite the webhook tests with the **real** shapes (G2/G3 + connect) so
  green means correct. Files: `internal/handler/unipile_*_test.go`,
  `internal/linkedin/unipile/unipile_test.go`.

---

## Unipile reference (facts learned the hard way)

- **API auth:** `X-API-KEY: <token>` header. Base URL = the DSN
  (`https://api47.unipile.com:17756`). Token created in dashboard →
  Access Tokens.
- **Webhook auth:** static custom header **only** (`Unipile-Auth: <secret>`).
  No HMAC body signing exists. Set the header when creating the webhook;
  handler compares it constant-time to `UNIPILE_WEBHOOK_SECRET`.
- **Webhook sources are separate** — register one per source:
  `account_status`, `users` (→ `new_relation`), `messaging` (→ new message).
  `POST /api/v1/webhooks` with `{source, request_url, name, format:"json",
  headers:[{key:"Unipile-Auth",value:<secret>}]}`.
- **Acceptance (`new_relation`) is delayed up to 8h** and carries
  `user_provider_id` / `user_public_identifier`, **no `invitation_id`**.
- **Messages** carry no `is_sender`; derive direction from
  `account_info.user_id` vs `sender.attendee_provider_id`.
- **Invites/DMs need a resolved `provider_id`** (member id), not a public URL.
- **Webhooks can't reach `.localhost`** → use a cloudflared tunnel for live
  testing (`cloudflared tunnel --url http://api-<devpod>.magiklead.localhost
  --http-host-header api-<devpod>.magiklead.localhost`). Quick-tunnel URLs are
  ephemeral — re-register webhooks after each restart.
- **LinkedIn re-invite rate limit:** re-inviting the same person soon returns
  `422 already_invited_recently` (recipient-specific, ~weeks). **Do not
  reject/withdraw test invites** — it burns that recipient.

**Docs:** webhooks overview `/docs/webhooks-2` · accepts
`/docs/detecting-accepted-invitations` · messages `/docs/new-messages-webhook`
· create-webhook ref `/reference/webhookscontroller_createwebhook`
(all under `https://developer.unipile.com`).

---

## How to test this properly

1. **Outbound** is live-testable and works (resolve → invite/DM).
2. **Inbound:** simulate with **real-shaped** payloads (above) signed with the
   `Unipile-Auth` header — the auth path now accepts them. This is the
   reliable way to test, given the 8h accept delay + invite rate limits make a
   true live accept impractical on demand.
3. Treat the **reconcile poll** (relations-list diff) as the path that must
   work, not the webhook.
4. Rebuild the webhook test fixtures from the documented shapes (G6).

---

## Current environment state / cleanup

- **`~/.config/devpods/magiklead/.env.backend`** now has real
  `UNIPILE_DSN` + `UNIPILE_API_KEY`. **Rotate the API key** — it was pasted
  into a chat transcript.
- **Uncommitted code change:** the G1 auth fix (keep) **plus a temporary
  `UNIPILE-CAPTURE` debug log** in `Webhook()` (**remove**). Touches
  `unipile.go`, `handler/unipile.go`, `handler/unipile_test.go`.
- **Manual DB row:** `linkedin_accounts` for tenant
  `be594ede-09be-4bb5-aa39-787710a1ec49`, `unipile_account_id =
  Fkiw0HOvQVqUUSuvd-HV_w` (re-verify it still exists). The auto-bind path is
  unproven (G5).
- **3 Unipile webhooks** registered against an **ephemeral cloudflared
  tunnel that is now dead** — re-register against a fresh tunnel before any
  live test.

---

## Suggested order

1. G1 cleanup: remove the temp log; commit the auth fix + a real-payload test.
2. G3: message direction (smallest correct fix; unblocks reply-halt).
3. G2: acceptance by provider_id — store provider_id at send, match
   `new_relation` + relations-list reconcile by it; rewrite
   `MarkLinkedInAccepted`.
4. G4: resolve provider_id on the outbound path.
5. G5: confirm + fix `account.connected` auto-bind.
6. G6: rebuild all webhook tests on real payload shapes; run full lifecycle
   (real send + simulated real-shape inbound).
7. Rotate the exposed Unipile API key.
