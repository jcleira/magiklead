# Spike captures — real Unipile payloads

Output of issue [#1 (validation spike)](./issues/01-validation-spike.md). These
are the **real** shapes Unipile sent/returned, captured against a live LinkedIn
account on 2026-07-01 and 2026-07-06. Personal data (names, member ids, URLs, message bodies)
is redacted with `<placeholders>`; **enum/structural values are kept verbatim**
because those are what the code routes on.

Issue #2 turns these into committed test fixtures (with anonymized values).

## Environment used (ephemeral — will not survive)

- Unipile DSN: `https://api55.unipile.com:18520` (token in `.env.backend`) — trial expired 2026-07-07, then **paid on the same workspace**: DSN + key unchanged (see the 2026-07-07 section)
- Connected account: `unipile_account_id = fFl27we2Rqa0eSbwIKX9hw` (jcleira / "Jose Corral", LINKEDIN)
- Tenant: `be594ede-09be-4bb5-aa39-787710a1ec49` ("jmc.leira's Workspace")
- Binding: **done manually** (`INSERT INTO linkedin_accounts …`) because auto-bind
  is broken — see finding A.
- Webhooks registered at Unipile → tunnel: `messaging` + `account_status`.

### 2026-07-06 session (cleanup + remaining read-only captures)

- **Residue removed**: the manual `linkedin_accounts` row
  (`90dff2d4-…`, tenant `be594ede-…`) deleted; both dead webhook
  registrations (pointing at the expired
  `province-lovers-cinema-wise.trycloudflare.com` tunnel) de-registered
  (`DELETE /api/v1/webhooks/<id>` → 200). The account is now **unbound**
  until #3 lands auto-bind.
- **Fresh registrations**: tunnel
  `https://occurred-decor-parallel-gzip.trycloudflare.com` →
  `account_status` id `P8BKt7x-T9GlzPsyi0Y6CQ`, `messaging`
  (events: `message_received`) id `qHkZKwd0RhyNJ9BtzKrzkQ`. End-to-end
  delivery verified: a probe POST through the tunnel reached the
  handler's `UNIPILE-CAPTURE` log (auth-rejected 401, captured anyway —
  the log fires before auth).
- **Re-registration recipe** (quick tunnels die with their process; re-run
  after every tunnel restart):

  ```sh
  # The SHARED devpods-traefik publishes only :80 (verified 2026-07-08 —
  # port 80 serves the app 200, no https listener). A 2026-07-07 restart
  # briefly exposed :443 with an http->https redirect, but that reverted;
  # target :80. (If a future restart brings :443 back, switch to
  # `--url https://localhost:443 --no-tls-verify`.)
  cloudflared tunnel --url http://localhost:80 \
    --http-host-header api-mvp.magiklead.localhost --no-autoupdate
  # then, per source (account_status | messaging):
  curl -X POST -H "X-API-KEY: $UNIPILE_API_KEY" -H 'Content-Type: application/json' \
    "$UNIPILE_DSN/api/v1/webhooks" -d '{"source":"account_status",
    "request_url":"https://<tunnel>.trycloudflare.com/api/v1/webhooks/unipile",
    "name":"magiklead-mvp-account_status",
    "headers":[{"key":"Unipile-Auth","value":"<UNIPILE_WEBHOOK_SECRET>"}]}'
  # messaging additionally takes: "events":["message_received"]
  ```

- **Key rotation still pending**: the leaked key verified **alive**
  today (`GET /api/v1/accounts` → 200). Rotation is dashboard-only.
  The key is confirmed absent from the working tree and git history —
  the leak was prior-session context only.
- **Rig re-verified (09:27):** tunnel process alive, probe POST through
  `occurred-decor-parallel-gzip.trycloudflare.com` reached the capture
  log (401 — logged before auth, as designed), and `GET /api/v1/webhooks`
  shows both registrations enabled and pointing at that tunnel. The
  capture log holds probes only — **no `message_received` has arrived
  yet**; the rig is armed and waiting on one live inbound DM.
- ~~Post-rotation verification~~ — superseded: the trial expired on
  2026-07-07 and the key is being **replaced** (new paid workspace),
  not rotated. See the 2026-07-07 section.
- `POST /api/v1/accounts/<id>/restart` → 200 `{"object":"AccountRestarted"}`
  (exists, non-destructive). A restart of a **healthy** account emitted
  **no** `account_status` webhook — transitions seem to fire only on real
  state changes. (`GET …/resync` does not exist: 404.)

### 2026-07-07 — trial over → paid on the SAME workspace; rig re-armed

- The trial expired and jcleira **subscribed on the existing
  workspace** — DSN and API key unchanged. The key in `.env.backend`
  is therefore still the **leaked** one → rotation is back to
  **required**, and now possible (the paid dashboard is functional).
- During the gap — while a fresh registration was the plan — the
  workspace was emptied as abandoned-trial hygiene: the LinkedIn
  account was disconnected (`DELETE /api/v1/accounts/fFl27we2Rqa0eSbwIKX9hw`
  → 200 `AccountDeleted`; the dashboard shows "Disconnected") and the
  07-06 webhook registrations were deleted. **The LinkedIn account must
  be re-connected** (hosted-auth via the app's connect screen) before
  any further capture/send work. All §1–§9 captures remain valid —
  same workspace, same shapes.
- A host/daemon restart around 12:00 on 07-06 had killed the capture
  tunnel AND the api/worker/frontend containers (`Exited (255)`; the
  infra containers auto-restarted). Minutes before dying, the rig
  caught the **organic inbound DM** frozen in §9.
- Rig re-armed 2026-07-07, then **re-pointed 2026-07-08**. On 07-07 a
  `devpods up` restart briefly flipped the shared Traefik to an
  http→https redirect (only :443 answered), so the tunnel was aimed at
  `https://localhost:443`. By 07-08 the shared `devpods-traefik` had
  restarted again and reverted to **:80-only** (no :443 listener at
  all — the app serves 200 on plain http), which killed that tunnel
  (origin refused). Current rig: tunnel
  `https://bronze-bigger-newspapers-mortgages.trycloudflare.com` →
  `http://localhost:80`; registrations `account_status` id
  `e15dxRuiRAmKMf88q-GdtA` + `messaging` (`message_received`) id
  `SsIIjznxSSmv5gyAzDf-TQ`; probe POST reached the capture log (401 →
  logged before auth). **Lesson: the shared Traefik's published ports
  are not stable across restarts** (other mag-family devpods cycle it);
  re-probe the origin after any restart before trusting the tunnel.

**Key rotation: waived by jcleira (2026-07-07)** — the key never
reached the repo or git history (session-context exposure only) and
the owner accepts keeping it on the paid workspace. With that
decision **#1 is closed**. Still pending operationally (not #1 ACs):
re-connect the LinkedIn account via the app's connect screen, and the
#3 `notify_url` connect capture.

## Auth (confirmed)

- Outbound: header `X-API-KEY: <token>` against the DSN base URL. `missing_credentials`
  (401) when the token is wrong/revoked — same body with or without the header.
- Inbound webhook: Unipile sends the fixed header we configured (`Unipile-Auth: <secret>`);
  our handler compares it constant-time. Verified live (a real webhook passed).

---

## 1. `account_status` webhook — account connected (real captured body)

Delivered to our webhook endpoint on connect completion. Routed in-handler on the
top-level `AccountStatus` key.

```json
{"AccountStatus":{"account_id":"fFl27we2Rqa0eSbwIKX9hw","message":"CREATION_SUCCESS","account_type":"LINKEDIN"}}
```

- Status vocabulary lives in `AccountStatus.message`. Seen: **`CREATION_SUCCESS`**.
  Other values (error/credentials/disconnect) **not yet captured** — need real
  transitions (issue #4).
- **No tenant metadata in this payload.** This is why auto-bind fails (finding A).

---

## 2. Connections — `GET /api/v1/users/relations?account_id=<acc>`

Accept-matcher input (issue #7). Envelope `{object, items[], cursor}`.

```json
{
  "object": "UserRelationsList",
  "items": [{
    "object": "UserRelation",
    "connection_urn": "urn:li:fsd_connection:<id>",
    "created_at": 1782818741000,
    "first_name": "<first>",
    "last_name": "<last>",
    "member_id": "ACoAA<...>",
    "member_urn": "urn:li:fsd_profile:ACoAA<...>",
    "headline": "<headline>",
    "public_identifier": "<slug>",
    "public_profile_url": "https://www.linkedin.com/in/<slug>/",
    "profile_picture_url": "<url>"
  }],
  "cursor": "<string|null>"
}
```

- **`member_id`** is the accept-matcher key — compare against each `awaiting_accept`
  lead's cached `linkedin_member_id`. `member_id` == the suffix of `member_urn`.
- `public_profile_url` links the member id back to the `linkedin_url` identifier.

## 3. Sent invitations — `GET /api/v1/users/invite/sent?account_id=<acc>`

Withdrawal-sweep input. Envelope `{object, items[], cursor}`.

```json
{
  "object": "InvitationList",
  "items": [{
    "object": "InvitationSent",
    "id": "7449262477600571392",
    "date": "Sent 2 months ago",
    "parsed_datetime": "2026-05-01T13:32:38.772Z",
    "invitation_text": null,
    "invited_user": "<name>",
    "invited_user_id": "ACoAA<...>",
    "invited_user_public_id": "<slug>",
    "invited_user_profile_picture_url": "<url>",
    "invited_user_description": "<headline>"
  }],
  "cursor": "<string|null>"
}
```

- **`id`** is the invitation id stored at send time and used by the withdrawal sweep.
- **`invited_user_id`** is the invitee's member id (same format as connections).

## 4. Chats — `GET /api/v1/chats?account_id=<acc>`

Reply-detection backup (issue #8). Envelope `{object, items[], cursor}`.

```json
{
  "object": "ChatList",
  "items": [{
    "object": "Chat",
    "name": null,
    "type": 0,
    "folder": ["INBOX", "INBOX_LINKEDIN_CLASSIC"],
    "pinned": 0, "unread": 0, "archived": 0, "read_only": 0,
    "timestamp": "2026-06-26T07:55:47.000Z",
    "account_id": "<unipile_account_id>",
    "muted_until": null,
    "provider_id": "2-<base64>",
    "account_type": "LINKEDIN",
    "unread_count": 0,
    "disabledFeatures": [],
    "attendee_provider_id": "ACoAA<...>",
    "id": "<chat_id>"
  }],
  "cursor": "<string|null>"
}
```

- **`id`** is the chat id (matches `campaign_leads.linkedin_chat_id`).
- **`attendee_provider_id`** is the other party's member id.
- **No direction field on the chat** — direction is per-message (see #5).

## 5. Messages — `GET /api/v1/chats/<chat_id>/messages`

Reply-direction source (issue #8). Envelope `{object, items[], cursor}`.

```json
{
  "object": "MessageList",
  "items": [{
    "object": "Message",
    "id": "<message_id>",
    "chat_id": "<chat_id>",
    "provider_id": "<...>",
    "chat_provider_id": "2-<base64>",
    "text": "<message body>",
    "is_sender": 0,
    "sender_id": "ACoAA<...>",
    "sender_attendee_id": "<...>",
    "account_id": "<unipile_account_id>",
    "message_type": "<...>",
    "attendee_type": "<...>",
    "attendee_distance": 1,
    "timestamp": "2026-06-26T07:55:46.625Z",
    "seen": 0, "delivered": 1, "edited": 0, "hidden": 0,
    "deleted": 0, "is_event": 0,
    "seen_by": {}, "reactions": [], "attachments": [],
    "subject": null, "behavior": null, "original": "<...>"
  }],
  "cursor": "<string|null>"
}
```

- **Direction:** `is_sender` — `0` = inbound (prospect reply), `1` = outbound (our
  account). Robust cross-check per the PRD: `sender_id` != the connected account's
  own member id ⇒ inbound.
- **Both directions captured** (2026-07-06, from a real April-outreach chat). The
  outbound example — note `sender_id` == the account's **own** member id:

```json
{
  "object": "Message",
  "id": "<message_id>",
  "chat_id": "<chat_id>",
  "provider_id": "2-<base64>",
  "chat_provider_id": "2-<base64>",
  "text": "<message body>",
  "is_sender": 1,
  "sender_id": "ACoAAAcdhtIB<own-member-id>",
  "sender_attendee_id": "<...>",
  "account_id": "fFl27we2Rqa0eSbwIKX9hw",
  "message_type": "MESSAGE",
  "attendee_type": "MEMBER",
  "attendee_distance": 1,
  "timestamp": "2026-04-12T19:09:31.566Z",
  "seen": 0, "delivered": 1, "edited": 0, "hidden": 0,
  "deleted": 0, "is_event": 0,
  "seen_by": {}, "reactions": [], "attachments": [],
  "subject": null, "behavior": null, "original": ""
}
```

- **`message_type` vocabulary seen**: `MESSAGE` (normal DM, both directions) and
  `INMAIL` (recruiter/sales InMail, seen inbound). Reply detection (#8) should
  treat any inbound item in a campaign chat as a reply regardless of type.

## 6. Resolve — `GET /api/v1/users/<identifier>?account_id=<acc>` (captured 2026-07-06)

Member-id resolver input (issue #5). `<identifier>` accepts the LinkedIn
**public identifier** (the `/in/<slug>/` path segment of a profile URL); the
member id comes back as **`provider_id`**. No envelope — a bare object:

```json
{
  "object": "UserProfile",
  "provider": "LINKEDIN",
  "provider_id": "ACoAA<...>",
  "public_identifier": "<slug>",
  "member_urn": "119375570",
  "first_name": "<first>",
  "last_name": "<last>",
  "headline": "<headline>",
  "primary_locale": {"country": "US", "language": "en"},
  "is_open_profile": false, "is_premium": false, "is_influencer": false,
  "is_creator": false, "is_relationship": false, "is_self": true,
  "websites": [],
  "follower_count": 438, "connections_count": 434,
  "location": "<location>",
  "contact_info": {"emails": ["<email>"]},
  "profile_picture_url": "<url>",
  "profile_picture_url_large": "<url>",
  "background_picture_url": "<url>"
}
```

- **`provider_id` is the member id** (`ACoAA…` — same key format as
  connections/invites/messages).
- Careful: `member_urn` here is **numeric** (`119375570`), unlike the
  connections-list `member_urn` (`urn:li:fsd_profile:ACoAA…`). Resolve on
  `provider_id`, not `member_urn`.
- Captured against the account's own profile (`is_self: true`); a
  non-self profile will additionally carry relationship/distance fields —
  shape for those not yet frozen, but #5 only needs `provider_id`.

## 7. Account object — `GET /api/v1/accounts` (captured 2026-07-06)

Where the connected account's **own member id** lives (the #8 direction
cross-check needs it): `connection_params.im.id`.

```json
{
  "object": "AccountList",
  "items": [{
    "object": "Account",
    "connection_params": {
      "im": {
        "id": "ACoAAAcdhtIB<own-member-id>",
        "publicIdentifier": "<slug>",
        "username": "<display name>",
        "connection_method": "credentials",
        "premiumId": null, "premiumFeatures": [], "premiumContractId": null,
        "organizations": [{"name": "<org>", "messaging_enabled": true,
          "mailbox_urn": "urn:li:fsd_pageMailbox:<id>",
          "organization_urn": "urn:li:fsd_company:<id>"}],
        "proxy": {"country": "ES"}
      }
    },
    "name": "<display name>",
    "type": "LINKEDIN",
    "created_at": "2026-07-01T12:50:22.378Z",
    "sources": [{"id": "<acc>_MESSAGING", "status": "OK"}],
    "id": "fFl27we2Rqa0eSbwIKX9hw",
    "groups": []
  }],
  "cursor": null
}
```

- `connection_params.im.id` == `provider_id` from resolve(`is_self`) — verified
  identical on the live account.
- `sources[].status` (`OK`) is the **API-side** account health, distinct from the
  `AccountStatus.message` webhook vocabulary (#4 maps the latter).

## 8. Messaging-webhook field list (from the registration object, 2026-07-06)

The webhook registration (`GET /api/v1/webhooks`) declares exactly which
keys the `messaging` source POSTs. Real bytes captured 2026-07-06 — see
**§9**; the declared list (below) and the real delivery differ in both
directions (§9 notes the diff):

```
account_id, account_type, account_info, webhook_name, chat_id, attendees,
sender, subject, message, mentions, message_id, timestamp, attachments,
reaction, reaction_sender, read_by, is_sender, provider_chat_id,
provider_message_id, is_event, chat_pinned, quoted, reply_to,
is_forwarded, chat_content_type, message_type, is_group, folder
```

- **`is_sender` is declared** on the webhook payload too — confirmed in the
  real bytes (§9), where it arrives as a **bool** (finding F).
- Confirmed from real bytes (§9): keys arrive **flat** at the top level.
  Declared-but-absent in the §9 delivery: `mentions`, `reaction`,
  `reaction_sender`, `read_by`. Present-but-undeclared: `event` (the routing
  marker) and `event_type` (`null`). Parsers must treat keys as conditional
  per event.

## 9. `message_received` webhook — real raw body (captured 2026-07-06 09:43)

An **organic** inbound cold-outreach DM arrived 14 minutes after the 07-06
rig went up — no staged second-profile DM was needed. Delivered with the
`Unipile-Auth` header (passed handler auth); the current parser then
**rejected it 400**: `json: cannot unmarshal bool into Go struct field
webhookPayload.is_sender of type int` (finding F). Raw unredacted bytes
(third-party PII) stashed outside the repo at
`~/.config/devpods/magiklead/captures/message_received-2026-07-06-0943.raw.json`
for #2's fixture anonymization.

```json
{
  "event": "message_received",
  "account_id": "fFl27we2Rqa0eSbwIKX9hw",
  "account_type": "LINKEDIN",
  "account_info": {"type": "LINKEDIN", "feature": "classic", "user_id": "ACoAA<own-member-id>"},
  "webhook_name": "magiklead-mvp-messaging",
  "chat_id": "<chat_id>",
  "attendees": [{
    "attendee_id": "<attendee_id>",
    "attendee_provider_id": "ACoAA<sender-member-id>",
    "attendee_name": "<name>",
    "attendee_profile_url": "https://www.linkedin.com/in/ACoAA<sender-member-id>",
    "attendee_specifics": {
      "provider": "LINKEDIN",
      "member_urn": "urn:li:member:<numeric>",
      "occupation": "<headline>",
      "is_company": false,
      "network_distance": "DISTANCE_1"
    },
    "attendee_public_identifier": null
  }],
  "sender": {
    "attendee_id": "<attendee_id>",
    "attendee_provider_id": "ACoAA<sender-member-id>",
    "attendee_name": "<name>",
    "attendee_profile_url": "https://www.linkedin.com/in/ACoAA<sender-member-id>",
    "attendee_specifics": {
      "provider": "LINKEDIN",
      "member_urn": "urn:li:member:<numeric>",
      "occupation": "<headline>",
      "is_company": false,
      "network_distance": "DISTANCE_1"
    },
    "attendee_public_identifier": null
  },
  "subject": null,
  "message": "<message body>",
  "message_id": "<message_id>",
  "timestamp": "2026-07-06T09:43:27.002Z",
  "attachments": [],
  "is_sender": false,
  "provider_chat_id": "2-<base64>",
  "provider_message_id": "2-<base64>",
  "is_event": 0,
  "chat_pinned": 0,
  "quoted": null,
  "reply_to": null,
  "is_forwarded": null,
  "chat_content_type": null,
  "message_type": "MESSAGE",
  "is_group": false,
  "folder": ["INBOX", "INBOX_LINKEDIN_CLASSIC"],
  "event_type": null
}
```

- **Marker confirmed:** top-level `"event": "message_received"` — the
  messaging counterpart of the status source's top-level `AccountStatus`
  key. (`event_type` is a separate field, `null` here.)
- **Types are NOT the REST types** (finding F): `is_sender` is a **bool**
  here vs int `0/1` on `GET /chats/<id>/messages` (§5); `is_group` is a
  bool; `is_event`/`chat_pinned` are ints; `quoted`/`reply_to`/
  `is_forwarded`/`chat_content_type`/`attendee_public_identifier` are
  nullable.
- **Direction on the webhook:** `is_sender: false` + the sender's member
  id at `sender.attendee_provider_id` (`ACoAA…` — finding C holds); the
  connected account's own member id rides along at `account_info.user_id`.
  Everything #8 needs is on the payload — no follow-up `/messages` fetch
  required.
- `sender` duplicates the matching `attendees[]` entry (1:1 chat).
- Yet another `member_urn` format: `urn:li:member:<numeric>` (connections
  §2: `urn:li:fsd_profile:ACoAA…`; resolve §6: bare numeric). Route on
  `ACoAA…` ids only, never on `member_urn`.

---

## Findings

- **A. Auto-bind is broken (→ issue #3).** The connect webhook (#1) carries **no
  tenant metadata**, and the hosted-auth request sets **no `notify_url`** — the
  `hostedAuthRequest` struct has no such field. Unipile therefore has no channel to
  return the signed `name` token that maps the account to a tenant. Fix: add
  `notify_url` to the hosted-auth link and bind on any payload carrying decodable
  metadata. Needs one live connect to capture the real `notify_url` payload shape.
- **B. Messages *do* carry `is_sender`** (the PRD was unsure). Direction is available
  from both `is_sender` and `sender_id` vs the account's own member id.
- **C. `member_id` is the universal LinkedIn key** — identical format
  (`ACoAA…`) across connections (`member_id`), invites (`invited_user_id`),
  messages (`sender_id`), resolve (`provider_id`), and the account object
  (`connection_params.im.id`). Confirms the member-id resolver/cache design (#5/#6).
- **D. Direction cross-check grounded on real bytes (2026-07-06).** On the live
  account: `is_sender: 1` ⟺ `sender_id` == `connection_params.im.id` (own member
  id), `is_sender: 0` ⟺ `sender_id` == the prospect's member id. Both signals agree
  on every message inspected.
- **E. Status webhooks fire on transitions only.** `POST /accounts/<id>/restart`
  of a healthy account emitted no `account_status` webhook. The
  non-`CREATION_SUCCESS` vocabulary (#4) will have to come from real transitions —
  the #3 reconnect flow is the next natural emitter (expect a reconnect-success
  value); restriction/credential values may not be capturable on demand.
- **F. Webhook types ≠ REST types — `is_sender` is a bool on the webhook,
  an int on REST.** The first real `message_received` delivery (§9) passed
  auth and was then **rejected 400** by the current int-typed
  `webhookPayload` struct (`cannot unmarshal bool into … is_sender`) —
  exactly the class of bug this spike existed to catch. #2/#8 must type the
  webhook surface from §9 (bool) and the REST surface from §5 (int)
  separately; one shared struct will break one side.

## Code fix already applied (uncommitted)

- `internal/linkedin/unipile/unipile.go`: hosted-auth `expiresOn` now formats with
  millisecond precision (`2006-01-02T15:04:05.000Z`); plain RFC3339 was rejected 400
  by Unipile. Without this, connect/reconnect 500s.

## Still open (not captured)

- `notify_url` connect payload (blocks #3; needs a live connect once `notify_url` is added).
- ~~`message_received` **webhook** raw bytes~~ — **captured 2026-07-06**
  (organic inbound DM; frozen in §9, raw stash outside the repo).
- `account_status` transitions other than `CREATION_SUCCESS`
  (restriction/disconnect/reconnect, #4) — not emittable on demand (finding E);
  capture opportunistically during the #3 reconnect.
- A non-self **resolve** response (relationship/distance fields) — cosmetic; #5
  only needs `provider_id`, which is frozen (§6).
