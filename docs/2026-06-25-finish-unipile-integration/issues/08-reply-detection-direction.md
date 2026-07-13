# 08 — Reply detection + correct direction

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Webhook auth + real-event routing](./02-webhook-auth-event-routing.md)

**Confirmed from spike (fill after #1):** direction-identifying fields
(connected account vs message sender) = `is_sender` (`0` inbound / `1`
outbound) cross-checked against `sender_id` == the account's own member
id (`connection_params.im.id` on the account object) — both verified on
real bytes, both directions (spike finding D; captures §5/§7); chats-list
exposes direction = **no** — per-message only (§4). The `message_received`
webhook carries direction directly (**§9**, real bytes): `is_sender` as
a **bool** (`false` on the captured inbound) — NOT the REST int (finding
F) — plus the sender's member id at `sender.attendee_provider_id` and
the account's own member id at `account_info.user_id`; no follow-up
`/messages` fetch is needed on the webhook path.

## What to build

Stop the sequence the **instant a prospect actually replies**, and
**never** halt on the operator's own outgoing message. Message
**direction** is derived from the **real captured fields** that identify
the connected account vs the message sender — not from a presumed
`is_sender` flag. A message from the connected account itself is
**ignored**; a message from the prospect is a **reply**. On a prospect
reply: match the lead by its stored chat id, flip it to `replied`, write
a `replied` event, and **suppress the person across all campaigns**. The
message notification (webhook) is the prompt path; the periodic
chats-list check is the backup. The self-halt bug is fixed **entirely**
by correct direction in the client — the suppression module does not
change.

## Acceptance criteria

- [x] Direction is derived from the **real fields** confirmed in #1
  (connected account vs sender), not a presumed `is_sender` flag.
- [x] A message from the connected account itself does **NOT** halt the
  sequence (self-halt bug fixed).
- [x] A prospect reply matches the lead by **stored chat id**, flips it
  to `replied`, writes a `replied` event, and suppresses the **person**
  across all campaigns (person-level suppression unchanged).
- [x] The chats-list **backup** path derives direction the **same way**
  as the notification path (if the chats-list exposes direction, per
  #1).
- [x] Handler **integration tests** drive the captured **inbound** +
  **outbound** messages: a prospect reply halts the lead + suppresses
  the person; our own message does **NOT** halt.

## Modules touched

- Unipile client direction logic
  (`backend/internal/linkedin/unipile/unipile.go` — `ParseWebhook`
  direction derivation; `AccountActivity` chats backup).
- Webhook handler (`backend/internal/handler/unipile.go` —
  `handleMessageReceived`).
- Worker reply reconcile (`backend/internal/worker/linkedin.go` —
  `applyLinkedInReply`) and `GetLinkedInLeadByChatID`.
- Suppression module (`backend/internal/suppression/suppression.go` —
  `RecordReplyByPerson`) — **unchanged**; the fix lives in the client's
  direction derivation.

## Test prior art

- `backend/internal/handler/unipile*reply*integration_test.go` —
  **rebuild on the real captured inbound + outbound messages**, asserting
  both the halt-on-reply and the no-halt-on-own-message outcomes.

## Out of scope

- The suppression module itself — unchanged (the fix is entirely correct
  direction in the client).
- Inbound email / mailto unsubscribe — a separate known operational gap,
  unchanged here.
