-- name: CreateEmailEvent :one
INSERT INTO email_events (campaign_lead_id, event_type, step, metadata, gmail_message_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListEmailEvents :many
SELECT * FROM email_events WHERE campaign_lead_id = $1 ORDER BY created_at;

-- name: FindEmailEventByGmailMessageID :one
SELECT * FROM email_events
WHERE gmail_message_id = $1
LIMIT 1;
