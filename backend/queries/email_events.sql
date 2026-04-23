-- name: CreateEmailEvent :one
INSERT INTO email_events (campaign_lead_id, event_type, step, metadata)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListEmailEvents :many
SELECT * FROM email_events WHERE campaign_lead_id = $1 ORDER BY created_at;
