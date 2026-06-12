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

-- ListEmailEventsForExport dumps every email_event tied to a tenant
-- (via campaign_leads → campaigns). Used by the GDPR account-export
-- endpoint (issue #11). Raw rows, oldest first.
-- name: ListEmailEventsForExport :many
SELECT ee.*
FROM email_events ee
JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
JOIN campaigns c       ON c.id  = cl.campaign_id
WHERE c.tenant_id = $1
ORDER BY ee.created_at;
