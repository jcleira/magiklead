-- name: CreateLinkedInEvent :one
-- One row per LinkedIn engine action against a campaign_lead. The
-- LinkedIn analogue of CreateEmailEvent; unipile_message_id holds the
-- invitation id on invite_sent and the message id on a DM, unipile_chat_id
-- is set once a chat exists (post-acceptance, issue #5).
INSERT INTO linkedin_events (campaign_lead_id, event_type, step, metadata, unipile_message_id, unipile_chat_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListLinkedInEvents :many
SELECT * FROM linkedin_events WHERE campaign_lead_id = $1 ORDER BY created_at;
