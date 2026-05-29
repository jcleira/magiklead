-- name: CreateGmailAccount :one
INSERT INTO gmail_accounts (tenant_id, user_id, email, access_token, refresh_token, token_expiry)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetGmailAccount :one
SELECT * FROM gmail_accounts WHERE id = $1 AND tenant_id = $2;

-- name: ListGmailAccounts :many
SELECT * FROM gmail_accounts WHERE tenant_id = $1;

-- name: UpdateGmailTokens :exec
UPDATE gmail_accounts SET access_token = $2, refresh_token = $3, token_expiry = $4 WHERE id = $1;

-- name: IncrementDailySent :exec
UPDATE gmail_accounts SET daily_sent_count = daily_sent_count + 1 WHERE id = $1;

-- name: ResetDailySentCounts :exec
UPDATE gmail_accounts SET daily_sent_count = 0, daily_sent_reset_at = NOW()
WHERE daily_sent_reset_at < CURRENT_DATE;

-- name: DeleteGmailAccount :exec
DELETE FROM gmail_accounts WHERE id = $1 AND tenant_id = $2;

-- name: UpdateGmailAccountCursor :exec
UPDATE gmail_accounts
SET last_history_id = $2, last_polled_at = $3
WHERE id = $1;

-- ListGmailAccountsForPolling returns every connected Gmail account
-- across all tenants — the reply-detection poller (issue #4) walks
-- this set on each 2-minute tick. Ordering by id keeps the rotation
-- through accounts deterministic across ticks so a slow account
-- doesn't block the same neighbours each cycle.
-- name: ListGmailAccountsForPolling :many
SELECT * FROM gmail_accounts ORDER BY id;
