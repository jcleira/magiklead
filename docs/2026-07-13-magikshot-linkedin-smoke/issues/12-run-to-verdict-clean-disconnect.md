# 12 — Run to verdict: daily observation, pass criteria, ≥1 reply, clean disconnect

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [11 — Campaign creation + manual start](./11-campaign-creation-manual-start.md)

Campaign start date: ______ (filled from #11's handoff)

## What to build

Operate the smoke daily per the runbook (#05) until an
evidence-based verdict, then leave no dangling access (PRD user
stories 24–27, 29, 30, 31). Campaign mechanics run as shipped —
this slice observes and records; it changes nothing.

1. **Daily observation.** Follow the runbook routine; append a
   dated entry per day to an evidence log in this docs folder
   (probes: sends vs. pacer allowance, lead-state counts, event
   timeline, tunnel/webhook liveness, worker loop health). Laptop
   downtime is an expected non-failure: the pacer resumes, the 45m
   reconcile poll back-fills — note the gap and move on.
2. **Accrue the remaining pass criteria** (4–7 + the win):
   - **Pacing held** — no day exceeds its warm-up-week allowance,
     weekly totals within the effective cap (SQL over
     `linkedin_events` by day).
   - **Acceptance advances to DM automatically** — via the
     relations webhook (fast path) or the reconcile poll
     (authoritative path; LinkedIn's notifications can lag ~8h).
     Evidence: an accepted lead's event timeline shows accept →
     first DM with no manual action.
   - **Reply halts and suppresses** — on a reply, the lead flips to
     `replied`, a person-keyed suppression row lands
     (`backend/internal/suppression/`), and zero automated messages
     follow, across campaigns (post-reply SQL sweep).
   - **Breaker, if it trips** — a fresh account pausing at <20%
     acceptance on ≥20 invites is plausible and is the protection
     working: capture the paused state in the UI/logs, document it
     as such, and let it govern (no override).
   - **Withdrawals only by the machine** — stale invites (21-day
     horizon) withdrawn by the hourly loop exclusively; **never by
     hand** (spike learning: rapid re-invite burns the recipient
     for weeks). Evidence: `withdrawn` events correlate with the
     loop's log lines.
   - **The win** — at least one genuine prospect reply recorded
     in-app (`replied` event carrying the inbound message id, from
     a real human).
3. **Clean disconnect** (pass criterion 7). When the founder calls
   the smoke complete: disconnect from the smoke pod's settings UI
   — `DELETE /api/v1/linkedin/accounts/{id}`
   (`UnipileHandler.DeleteAccount`,
   `backend/internal/handler/unipile.go`) revokes the Unipile
   session (`Module.Disconnect` → `DELETE /api/v1/accounts/{id}`)
   and unbinds the local row. Verify: 204; Unipile accounts list
   empty (curl with `X-API-KEY`); `linkedin_accounts` row gone.
4. **Verdict.** A final section in the evidence log: the seven
   criteria as a table, each green/red with a link to its evidence,
   plus the reply win — the smoke passes only if all seven are
   green and ≥1 genuine reply landed. The verdict doubles as the
   dress-rehearsal report for the real production launch.

## Acceptance criteria

- [ ] `Campaign start date:` filled; dated observation entries
      exist for the campaign window (gaps only from documented
      downtime).
- [ ] Criterion 4 evidence: per-day send counts never exceed the
      warm-up allowance; weekly totals within the effective cap.
- [ ] Criterion 5 evidence: ≥1 acceptance advanced to DM
      automatically (timeline captured; webhook or reconcile —
      either counts).
- [ ] Criterion 6 evidence: reply → immediate halt + cross-campaign
      suppression; zero automated messages to that person
      afterwards.
- [ ] ≥1 genuine prospect reply recorded in-app (the win).
- [ ] If the breaker tripped: paused state captured and documented
      as protection, not failure; no manual override occurred.
- [ ] Zero manual invite withdrawals; any withdrawals trace to the
      hourly loop.
- [ ] Clean disconnect verified three ways: 204 response, empty
      Unipile accounts list, no local `linkedin_accounts` row.
- [ ] Verdict table committed: seven criteria + reply win, each
      with linked evidence; overall pass/fail stated plainly.

## Modules touched

None (execution slice). Evidence log + verdict in
`docs/2026-07-13-magikshot-linkedin-smoke/`.

## Test prior art

The integration suites assert the same state transitions the
evidence queries must show:
`backend/internal/handler/unipile_accept_integration_test.go`,
`…unipile_reply_integration_test.go`,
`backend/internal/worker/linkedin_reconcile_integration_test.go`.
The e2e crafted-payload suite keeps covering the webhook spine in
CI — this slice adds the live-traffic proof CI cannot.

## Out of scope

- Fixing anything the smoke surfaces beyond operational recovery
  (a real defect becomes its own issue/PR — the verdict records
  it).
- Scaling the list, adding accounts, send windows, or the
  production deployment (PRD out of scope; the release phase reuses
  what this smoke built).
