# Issues — 2026-06-25-finish-unipile-integration

Source: [prd.md](./prd.md)

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Validation spike — capture real Unipile payloads, freeze fixtures](./issues/01-validation-spike.md) — captures of record: [spike-captures.md](./spike-captures.md); event marker: top-level `AccountStatus` key (status) vs top-level `event` field on a flat body (messaging, §9); status vocab seen: `CREATION_SUCCESS` only; direction: `is_sender` + `sender_id` vs own member id — webhook `is_sender` is **bool** vs int on REST (finding F); **auto-bind: no** (→ #3). **Complete 2026-07-07** — `message_received` captured 2026-07-06 (organic inbound DM → §9); key rotation waived by owner (exposure was session-context only; never repo/git). | None |
| [x]  | 2 | [Webhook auth + real-event routing](./issues/02-webhook-auth-event-routing.md) | [#1](./issues/01-validation-spike.md) |
| [x]  | 3 | [Connect-and-bind from real payload](./issues/03-connect-and-bind.md) | [#2](./issues/02-webhook-auth-event-routing.md) |
| [x]  | 4 | [Account status mapping](./issues/04-account-status-mapping.md) | [#2](./issues/02-webhook-auth-event-routing.md) |
| [x]  | 5 | [Member-id resolver](./issues/05-member-id-resolver.md) | [#1](./issues/01-validation-spike.md) |
| [x]  | 6 | [Invite + DM addressed by member id](./issues/06-send-by-member-id.md) | [#5](./issues/05-member-id-resolver.md) |
| [x]  | 7 | [Accept matcher + connections check](./issues/07-accept-matcher-connections-check.md) | [#6](./issues/06-send-by-member-id.md) |
| [x]  | 8 | [Reply detection + correct direction](./issues/08-reply-detection-direction.md) | [#2](./issues/02-webhook-auth-event-routing.md) |
| [x]  | 9 | [Dashboard reflects real inbound outcomes](./issues/09-dashboard-reflects-outcomes.md) | [#7](./issues/07-accept-matcher-connections-check.md), [#8](./issues/08-reply-detection-direction.md) |

Dependency shape: **#1 (validation spike) gates everything** — it freezes the real fixtures and confirms the headline unknowns. Two spines fan out after it: the webhook side **#2 → {#3, #4, #8}** and the outbound side **#5 → #6 → #7** (#5 can start as soon as #1 lands, in parallel with #2). Both spines converge at **#9**.

> **Spike handoff (#1) feeds the rest.** When #1 completes, record its frozen facts — the real event marker, the status vocabulary, the message-direction fields, the resolve/connections-list shapes, and whether auto-bind worked with zero SQL. Each downstream slice has a `Confirmed from spike` field to fill from those facts before it starts.

---

## Discovered 2026-07-09 (devpod test pass)

Surfaced while manually exercising connect on the `mvp` devpod. Not slices
above — recorded here so they aren't lost.

- **PREREQ (scoped) — the devpod can't receive *real* Unipile webhooks.**
  `api-<devpod>.magiklead.localhost` is unresolvable from Unipile's cloud,
  so live Unipile-originated events (`account.connected`, message/status)
  never arrive on the devpod. This does **not** block *building or
  verifying* the inbound spine (#2, #3, #4, #7, #8, #9): the handlers are
  exercisable locally by POSTing crafted payloads — from the frozen
  captures in [spike-captures.md](../spike-captures.md) — to
  `/api/v1/webhooks/unipile` with the right auth header + signed metadata
  (the unit/e2e suite already does exactly this). The tunnel is needed
  **only** for a real end-to-end smoke (actual Unipile → app), which is a
  nice-to-have, not a slice blocker. Interim connect workaround used this
  session: manual `INSERT INTO linkedin_accounts (tenant_id,
  unipile_account_id, status)` — evaporates on `devpods seed`.

- **#3 note — the bind code path exists but is unverified end-to-end.**
  `handler.handleAccountConnected` already upserts `status='active'` from
  the signed metadata; what's unproven is #2's routing + metadata echo
  actually reaching it (the spike logged "auto-bind: no"). Blocked on the
  webhook prereq above before it can be confirmed.
  **RESOLVED 2026-07-09 (#3 done).** `notify_url` is now wired onto the
  hosted-auth request (finding A) — verified against Unipile's docs, the
  flat `{status,account_id,name}` callback `name` echoes our signed
  metadata. Connect-and-bind is proven end-to-end by crafted-payload
  integration tests against the real DB (`unipile_connect_integration_test.go`:
  fresh-insert bind, surfaces-active via `ListAccounts`, no-metadata-status
  does-not-bind) plus client/handler `notify_url` unit tests. The only thing
  still needing the tunnel is a real Unipile→app smoke (nice-to-have).

- **Connect "Configuring" UX — DONE 2026-07-09 (frontend, uncommitted).**
  On return from hosted-auth (`/settings?linkedin=connected`) the UI now
  shows Finishing → Connected / timeout instead of a misleading "not
  connected" during the webhook-bind gap. **Open follow-up:** make it
  server-authoritative — write a `connecting` row at connect time so state
  survives refresh and doesn't lean on a client poll. Pairs with #3.

- **Visible login — DONE 2026-07-09 (frontend, uncommitted).** App shell
  rendered a dead placeholder instead of a user menu; replaced with Clerk
  `<UserButton>` + `afterSignOutUrl` on `ClerkProvider`. Not a Unipile
  slice; noted so it isn't lost.
