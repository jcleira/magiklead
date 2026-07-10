-- name: UpsertLinkedInAccount :one
-- Idempotent connect ingest: a replayed account.connected webhook for
-- the same Unipile account updates status in place rather than inserting
-- a duplicate. tenant_id is fixed on first insert and never reassigned
-- on conflict, so a replayed webhook can't move an account between
-- tenants.
INSERT INTO linkedin_accounts (tenant_id, unipile_account_id, status)
VALUES ($1, $2, $3)
ON CONFLICT (unipile_account_id) DO UPDATE
SET status = EXCLUDED.status, last_error = NULL
RETURNING *;

-- name: GetLinkedInAccount :one
SELECT * FROM linkedin_accounts WHERE id = $1 AND tenant_id = $2;

-- name: ListLinkedInAccounts :many
SELECT * FROM linkedin_accounts WHERE tenant_id = $1 ORDER BY created_at;

-- ListLinkedInAccountsForReconcile returns every account the reconcile
-- poll should sweep — those that can still act (active / warming).
-- Platform-wide (not tenant-scoped), the LinkedIn analogue of
-- ListGmailAccountsForPolling. Restricted / disconnected / connecting
-- accounts can't yield new accepts or replies, so they're skipped.
-- name: ListLinkedInAccountsForReconcile :many
SELECT * FROM linkedin_accounts
WHERE status IN ('active', 'warming')
ORDER BY created_at;

-- name: CountConnectedLinkedInAccounts :one
-- The free-tier connect gate: counts only accounts that occupy the one
-- account slot — connecting, active, or warming. An unhealthy account
-- (restricted or disconnected) does NOT block the user from re-running the
-- connect flow to recover it (issue #8): reconnecting the same account
-- upserts it back to active in place (ON CONFLICT on unipile_account_id),
-- so the gate must let the reconnect auth-url through rather than 403 it.
SELECT count(*) FROM linkedin_accounts
WHERE tenant_id = $1 AND status NOT IN ('restricted', 'disconnected');

-- name: SetLinkedInAccountStatusByUnipileID :exec
UPDATE linkedin_accounts SET status = $2, last_error = $3
WHERE unipile_account_id = $1;

-- name: DeleteLinkedInAccount :exec
DELETE FROM linkedin_accounts WHERE id = $1 AND tenant_id = $2;

-- IncrementLinkedInInviteCounters records one sent invite against the
-- account's rolling caps, resetting a window in the same statement when
-- it has rolled over. Both timestamps are window-START markers: the
-- weekly window runs [weekly_window_started_at, +7d) and the daily window
-- runs [daily_reset_at, +24h). When the current window is NULL or older
-- than its span, this send opens a fresh window (count = 1); otherwise it
-- bumps the count. The pacer (internal/linkedin/pacer) reads these same
-- fields with the same window semantics to decide allowance, so writer
-- and reader never disagree. Counting in the DB keeps a within-tick burst
-- correct: each send updates the row, so re-reading before the next send
-- sees the new count.
-- name: IncrementLinkedInInviteCounters :exec
UPDATE linkedin_accounts SET
    weekly_window_started_at = CASE
        WHEN weekly_window_started_at IS NULL
          OR weekly_window_started_at < NOW() - INTERVAL '7 days'
        THEN NOW() ELSE weekly_window_started_at END,
    weekly_invite_count = CASE
        WHEN weekly_window_started_at IS NULL
          OR weekly_window_started_at < NOW() - INTERVAL '7 days'
        THEN 1 ELSE weekly_invite_count + 1 END,
    daily_reset_at = CASE
        WHEN daily_reset_at IS NULL
          OR daily_reset_at < NOW() - INTERVAL '1 day'
        THEN NOW() ELSE daily_reset_at END,
    daily_invite_count = CASE
        WHEN daily_reset_at IS NULL
          OR daily_reset_at < NOW() - INTERVAL '1 day'
        THEN 1 ELSE daily_invite_count + 1 END
WHERE id = $1;

-- StartLinkedInWarmup anchors an account's warmup ramp the first time the
-- worker sends from it: warmup_started_at is set to NOW() only while still
-- NULL (COALESCE), so it is fixed once and never reset. The connect handler
-- (issue #1) lands accounts active without a warmup anchor, so the send tick
-- stamps it here — "warmup begins at first outreach" — and the pacer's ramp
-- (internal/linkedin/pacer) reads it from then on. Idempotent: a no-op once set.
-- name: StartLinkedInWarmup :exec
UPDATE linkedin_accounts
SET warmup_started_at = COALESCE(warmup_started_at, NOW())
WHERE id = $1;

-- GetLinkedInAcceptanceStats returns one account's trailing-7-day invite
-- outcomes straight from linkedin_events — invites sent vs invites accepted
-- — the raw signal the pacer's acceptance-rate breaker (issue #7) divides.
-- Events are keyed by campaign_lead, so we join back to the lead to scope
-- them to this account. The 7-day window matches the breaker's trailing
-- window; counting in SQL keeps the pure pacer free of any DB of its own.
-- name: GetLinkedInAcceptanceStats :one
SELECT
    COUNT(*) FILTER (WHERE e.event_type = 'invite_sent') AS invites_sent,
    COUNT(*) FILTER (WHERE e.event_type = 'accepted')    AS accepted
FROM linkedin_events e
JOIN campaign_leads cl ON cl.id = e.campaign_lead_id
WHERE cl.linkedin_account_id = $1
  AND e.created_at >= NOW() - INTERVAL '7 days';

-- SetLinkedInAcceptanceState records the acceptance-rate breaker's verdict
-- for an account (issue #7): whether its invites are paused and the computed
-- trailing-7-day rate behind that call, so the UI can show both the pause
-- and the number that caused it. The worker writes it only when the paused
-- flag flips, so there is no per-tick churn.
-- name: SetLinkedInAcceptanceState :exec
UPDATE linkedin_accounts
SET acceptance_paused = $2, acceptance_rate = $3
WHERE id = $1;
