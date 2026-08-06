# 05 — Smoke runbook + warm-up protocol + seven pass criteria

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately. Draft now, finalize
pointers (hostname, script names, override variable) as #01–#04
land. Must be complete before the ceremony (#06) uses it.

## What to build

`runbook.md` in this docs folder (warm-up protocol as a section or
sibling file — executor's call): the single document from which any
future session can run and judge the smoke without re-deriving it
(PRD user stories 7, 28, 29). Contents:

1. **Day-0 checklist.** Pod healthy and never-seed guardrail
   acknowledged (#01), tunnel service up + public reachability
   probe, `APP_URL` override in effect (`devpods exec api printenv
   APP_URL`), key rotated + clean account slate (#04), webhook
   registrations listed (3 sources — #03's CLI), founder signed up
   and onboarded in the smoke pod (Clerk dev keys; the JWT template
   + admin-email prereqs from CLAUDE.md apply).

2. **The connect ceremony script** (executed in #06): founder
   account-creation pointer (→ warm-up protocol), settings page →
   Connect LinkedIn → hosted-auth wizard, expected redirect
   (`?linkedin=connected`), what to watch: `devpods logs api` for
   `POST /api/v1/webhooks/unipile` → 200, the `linkedin_accounts`
   bind row appearing with zero manual SQL, the settings UI polling
   `/api/v1/linkedin/accounts` to connected.

3. **Approval gates.** The three founder gates — list before
   loading (#09), copy before campaign exists (#10), campaign start
   (#11) — who approves, where each is recorded.

4. **Daily observation routine.** Concrete probes, no vibes:
   - Sends vs. allowance: SQL over `linkedin_events` by day; the
     pacer's constants are code-defined in
     `backend/internal/linkedin/pacer/pacer.go` — week-1 daily cap
     8, ramping +4/week to 20, standard weekly cap 100, breaker at
     acceptance rate < 0.20 once ≥ 20 invites in the 7-day window.
   - Lead-state counts (`awaiting_accept` / accepted / `replied` /
     `not_accepted`) and event timeline (`accepted`, `replied`,
     `withdrawn`, `skipped` events).
   - Loop cadences to expect in worker logs: send tick 60s,
     reconcile 45m (the authoritative acceptance/reply backstop —
     LinkedIn notifications can lag ~8h), withdrawal hourly with a
     21-day stale horizon.
   - UI spots: capacity widget, account status, campaign metrics.
   - Webhook liveness: tunnel service status + last inbound POST in
     api logs; recovery = re-run #03's CLI (`list`/`register`).

5. **Seven pass criteria**, each mapped to its verification (the
   PRD's success definition):
   1. Webhook-bound connect with zero manual SQL — api log line +
      `linkedin_accounts` row provenance.
   2. Loaded prospects searchable — lead search returns curated
      people with LinkedIn URLs attached.
   3. Campaign created from approved copy — DB steps diffed against
      the approved copy file.
   4. Pacing held — no day exceeds the warm-up allowance; weekly
      total within the effective cap.
   5. Acceptance advances to DM automatically — event timeline
      shows accept (webhook or reconcile) then DM send.
   6. Reply halts and suppresses — lead `replied`, person-keyed
      suppression row, zero automated messages after the reply
      across campaigns.
   7. Clean disconnect — 204 from the in-app disconnect, Unipile
      accounts list empty, local row gone.
   Plus the win condition: ≥ 1 genuine prospect reply recorded
   in-app.

6. **Expected non-failures** (PRD Further Notes): ~8h acceptance
   lag (reconcile covers it); laptop downtime pauses sending — the
   pacer resumes and reconcile back-fills; the acceptance breaker
   pausing a fresh account on a small sample is the protection
   working, not a failure. Never withdraw invites by hand —
   re-inviting soon after burns the recipient for weeks (spike
   learning); the hourly loop owns withdrawal.

7. **Warm-up protocol.** New-account creation guidance (founder's
   real identity, profile photo generated with magikshot), manual
   activity cadence for 2–3 weeks (profile completeness, daily
   normal browsing, a handful of organic connects to known
   contacts, no automation), and an explicit **completion
   criterion** (e.g. ≥ 2 weeks elapsed + organic-connection floor +
   zero restriction warnings) that #11's go-gate checks against.

## Acceptance criteria

- [x] `runbook.md` committed with all seven sections above.
- [x] Every pass criterion names its probe concretely (SQL against
      real tables, a log pattern, a UI location, or a curl) — each
      verified to reference symbols/tables that exist in the code,
      not invented ones.
- [x] Warm-up protocol includes an unambiguous completion criterion
      a future session can evaluate.
- [x] Self-containment check: a fresh session given only the
      runbook + the repo could execute day-0 and one daily
      observation. (Per project memory: all criteria are
      curl/DB/log-verifiable; only founder actions are human.)

## Modules touched

- `docs/2026-07-13-magikshot-linkedin-smoke/runbook.md` (new). Docs
  only — no code.

## Test prior art

N/A. The runbook's probes should mirror how the integration suites
assert state (`backend/internal/handler/unipile_*_integration_test.go`
and `backend/internal/worker/` tests read the same tables).

## Out of scope

- Executing the ceremony (#06), the campaign (#11), or the daily
  routine (#12) — this issue only writes the book they follow.
