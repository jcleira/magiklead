-- Queries that back the suppression deep module
-- (backend/internal/suppression). IsSuppressed reads from three
-- sources: the unsubscribes table (tenant-specific or global), and
-- email_events bounce signals reachable from campaign_leads that
-- resolve to the candidate email.
--
-- Email resolution joins through campaign_leads → (persons + emails)
-- or → leads (legacy), case-insensitive, so the suppression module
-- doesn't care which ingest path populated the address.

-- name: LookupSuppression :one
-- Returns the suppression reason if (tenant_id, email) is suppressed,
-- prefering a tenant-specific row over a global one.
SELECT reason
FROM unsubscribes
WHERE (tenant_id = sqlc.arg(tenant_id) OR tenant_id IS NULL)
  AND lower(email) = lower(sqlc.arg(email)::text)
ORDER BY (tenant_id IS NULL) ASC
LIMIT 1;

-- name: InsertUnsubscribeTenant :exec
-- Idempotent insert of a tenant-scoped suppression row.
INSERT INTO unsubscribes (tenant_id, email, reason)
VALUES (sqlc.arg(tenant_id), sqlc.arg(email), sqlc.arg(reason))
ON CONFLICT (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(email)) DO NOTHING;

-- name: InsertUnsubscribeGlobal :exec
-- Idempotent insert of a global (tenant_id = NULL) suppression row.
INSERT INTO unsubscribes (tenant_id, email, reason)
VALUES (NULL, sqlc.arg(email), sqlc.arg(reason))
ON CONFLICT (COALESCE(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(email)) DO NOTHING;

-- name: HasHardBounceEvent :one
-- True if any campaign_lead for this tenant that resolves to this
-- email has a 'bounced' email_event row.
SELECT EXISTS (
    SELECT 1
    FROM email_events ee
    JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
    JOIN campaigns c       ON c.id  = cl.campaign_id
    LEFT JOIN persons p    ON p.id  = cl.person_id
    LEFT JOIN emails em    ON em.person_id = p.id
    LEFT JOIN leads l      ON l.id  = cl.lead_id
    WHERE c.tenant_id = sqlc.arg(tenant_id)
      AND ee.event_type = 'bounced'
      AND (lower(em.email) = lower(sqlc.arg(email)::text) OR lower(l.email) = lower(sqlc.arg(email)::text))
);

-- name: CountConsecutiveSoftBounces :one
-- Returns the count of 'soft-bounce' email_events since the last
-- successful 'sent' event for any campaign_lead in this tenant that
-- resolves to this email. "Consecutive" = no successful send has
-- reset the streak.
WITH cl_match AS (
    SELECT cl.id
    FROM campaign_leads cl
    JOIN campaigns c    ON c.id = cl.campaign_id
    LEFT JOIN persons p ON p.id = cl.person_id
    LEFT JOIN emails em ON em.person_id = p.id
    LEFT JOIN leads l   ON l.id = cl.lead_id
    WHERE c.tenant_id = sqlc.arg(tenant_id)
      AND (lower(em.email) = lower(sqlc.arg(email)::text) OR lower(l.email) = lower(sqlc.arg(email)::text))
),
last_sent AS (
    SELECT COALESCE(max(created_at), '1970-01-01'::timestamptz) AS at
    FROM email_events
    WHERE campaign_lead_id IN (SELECT id FROM cl_match)
      AND event_type = 'sent'
)
SELECT COUNT(*)::bigint
FROM email_events
WHERE campaign_lead_id IN (SELECT id FROM cl_match)
  AND event_type = 'soft-bounce'
  AND created_at > (SELECT at FROM last_sent);

-- name: FindCampaignLeadForEmail :one
-- Returns the most-recently-created campaign_lead in this tenant that
-- resolves to this email. Used by bounce recorders that take only
-- (email, tenant) and need a campaign_lead_id to anchor the event row.
SELECT cl.id
FROM campaign_leads cl
JOIN campaigns c    ON c.id = cl.campaign_id
LEFT JOIN persons p ON p.id = cl.person_id
LEFT JOIN emails em ON em.person_id = p.id
LEFT JOIN leads l   ON l.id = cl.lead_id
WHERE c.tenant_id = sqlc.arg(tenant_id)
  AND (lower(em.email) = lower(sqlc.arg(email)::text) OR lower(l.email) = lower(sqlc.arg(email)::text))
ORDER BY cl.created_at DESC
LIMIT 1;

-- name: ResolveCampaignLeadAddress :one
-- Returns (tenant_id, email) for a given campaign_lead, using the same
-- canonical/legacy resolution as GetDueLeads.
WITH best AS (
    SELECT DISTINCT ON (em.person_id) em.person_id, em.email
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
)
SELECT
    c.tenant_id,
    COALESCE(b.email, l.email, '')::text AS email
FROM campaign_leads cl
JOIN campaigns c       ON c.id = cl.campaign_id
LEFT JOIN persons p    ON p.id = cl.person_id
LEFT JOIN best b       ON b.person_id = p.id
LEFT JOIN leads l      ON l.id = cl.lead_id
WHERE cl.id = $1;

-- name: MarkCampaignLeadReplied :exec
UPDATE campaign_leads
SET status = 'replied', last_replied_at = NOW()
WHERE id = $1;

-- ClearReplyUnsubscribe deletes the reply-reason suppression row for
-- (tenant, email). Used by the re-engage handler when the operator
-- decides a reply was actually an auto-responder (out-of-office,
-- vacation-reply) and wants the sequence to resume. Other reasons
-- (manual / list-unsub / spam-complaint / hard-bounce / soft-bounce-
-- threshold) are deliberately NOT touched — only the reply gate is
-- recoverable; the others are permanent for safety / compliance.
-- name: ClearReplyUnsubscribe :exec
DELETE FROM unsubscribes
WHERE tenant_id = sqlc.arg(tenant_id)
  AND lower(email) = lower(sqlc.arg(email)::text)
  AND reason = 'reply';

-- ListUnsubscribesForTenant dumps every tenant-scoped unsubscribe row
-- (excludes globals where tenant_id IS NULL). Used by the GDPR
-- account-export endpoint (issue #11) to write unsubscribes.json.
-- name: ListUnsubscribesForTenant :many
SELECT * FROM unsubscribes
WHERE tenant_id = sqlc.arg(tenant_id)
ORDER BY created_at;

-- ReactivateCampaignLead is the second half of the re-engage flow:
-- after ClearReplyUnsubscribe lifts the suppression, this flips the
-- lead back to 'active' and arms next_send_at so the worker picks it
-- up on the next tick. Step is left unchanged — the sequence
-- continues from where it was when the reply landed.
-- name: ReactivateCampaignLead :exec
UPDATE campaign_leads
SET status = 'active', next_send_at = NOW(), last_replied_at = NULL
WHERE id = $1;
