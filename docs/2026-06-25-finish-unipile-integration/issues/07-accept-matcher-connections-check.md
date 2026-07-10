# 07 — Accept matcher + connections check

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [06 — Invite + DM addressed by member id](./06-send-by-member-id.md)

**Confirmed from spike (fill after #1):** connections-list response
shape = `GET /api/v1/users/relations?account_id=<acc>` →
`UserRelationsList{items[]{member_id, member_urn, public_identifier,
public_profile_url, connection_urn, created_at, …}, cursor}` — matcher
key is **`member_id`**. Frozen in
[spike-captures §2](../spike-captures.md).

## What to build

Detect acceptance by the **periodic (~45 min) connections check** —
never by the `new_relation` notification (8-hour lag). The Unipile
client gains a **list-connections** op (account → the member ids
currently connected). Each cycle, for each active/warming account, the
**accept matcher** returns which `awaiting_accept` leads — scoped to
that account — have a cached member id now present in the connections
set. On a match, flip the lead to `active`, stamp `accepted_at`, set
`current_step = 1`, schedule the **first DM immediately**, and write an
`accepted` event. Safe to run repeatedly — every transition is gated on
the current lead state.

## Acceptance criteria

- [x] The Unipile client exposes `list-connections`: account → set of
  connected member ids, built/tested against the **real connections-list
  shape** from #1.
- [x] The 45-min reconcile lists connections per active/warming account;
  the accept matcher returns exactly the `awaiting_accept` leads (scoped
  to that account) whose cached member id is in the set.
- [x] "Mark accepted" is rewritten to match a lead **by member id**
  (joining the lead's person to its `linkedin_member_id` identifier),
  scoped to the lead's bound account, gated on `status =
  'awaiting_accept'`.
- [x] On match: lead → `active`, `accepted_at` stamped, `current_step =
  1`, first DM scheduled immediately, `accepted` event written.
- [x] **Idempotent:** re-running a cycle over an already-accepted lead
  changes nothing (no double-message); no-match and already-accepted
  cases handled.
- [x] **Accept-matcher pure-logic unit tests:** feed a connections set +
  a set of `awaiting_accept` leads, assert exactly which flip
  (match / no-match / already-accepted).
- [x] The invitation id is **still stored at send time** and **still
  used** by the stale-invite withdrawal sweep — verify the sweep still
  works after the acceptance key moved from invitation id to member id.
  *Verified: `MarkLinkedInInviteSent` / `GetStaleLinkedInInvites` /
  `MarkLinkedInNotAccepted` untouched; the three
  `TestProcessLinkedInWithdrawals_*` integration tests pass unchanged.*
- [x] (Optional) A supporting index for the accept-match query (fetch
  `awaiting_accept` per account + join cached member id) — decide during
  implementation; not a correctness requirement at trial volume.
  *Decided: **skipped.** At trial volume the `(status, linkedin_account_id)`
  filter on `campaign_leads` is selective and the `person_identifiers` join
  rides the existing `idx_person_identifiers_person_id` /
  `..._type` indexes; no new index warranted.*

## Modules touched

- Worker reconcile (`backend/internal/worker/linkedin.go` —
  `reconcileLinkedInOnce`, `applyLinkedInAccept`) + a pure accept-matcher
  helper.
- Unipile client `list-connections`
  (`backend/internal/linkedin/unipile/unipile.go`).
- `MarkLinkedInAccepted` rewritten to key on member id
  (`backend/queries/campaign_leads.sql` + generated repository).
- Stale-invite sweep (`GetStaleLinkedInInvites` /
  `MarkLinkedInNotAccepted`) — unchanged; verify it still keys on
  `linkedin_invitation_id`.

## Test prior art

- `backend/internal/worker/*reconcile*integration_test.go`.
- `backend/internal/handler/unipile*accept*integration_test.go`
  (lifecycle) — **rebuild on the real fixtures**.
- Pure accept-matcher table tests (no external dependencies).

## Out of scope

- The `new_relation` notification path — deliberately **not** built; the
  connections check is the sole acceptance path.
- Optimizing the connections list for very large accounts — later
  follow-up; pull the list as-is for now.
- Reply detection — **#8**.

This slice also realizes US 10 (the acceptance breaker now reads a
**real** 7-day accept rate, because acceptances are finally recorded)
and US 24 (withdrawal preserved).
