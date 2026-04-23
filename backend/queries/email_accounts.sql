-- name: CreateEmailAccount :one
INSERT INTO email_accounts (tenant_id, user_id, provider, email, smtp_host, smtp_port, smtp_username, smtp_password, access_token, refresh_token, token_expiry, sender_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetEmailAccount :one
SELECT * FROM email_accounts WHERE id = $1 AND tenant_id = $2;

-- name: ListEmailAccounts :many
SELECT * FROM email_accounts WHERE tenant_id = $1 AND is_active = TRUE;

-- name: DeleteEmailAccount :exec
DELETE FROM email_accounts WHERE id = $1 AND tenant_id = $2;

-- name: UpdateEmailAccountTokens :exec
UPDATE email_accounts SET access_token = $2, refresh_token = $3, token_expiry = $4 WHERE id = $1;

-- name: IncrementEmailSent :exec
UPDATE email_accounts SET daily_sent_count = daily_sent_count + 1 WHERE id = $1;

-- name: ResetEmailSentCounts :exec
UPDATE email_accounts SET daily_sent_count = 0, daily_sent_reset_at = NOW()
WHERE daily_sent_reset_at < CURRENT_DATE;
