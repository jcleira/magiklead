-- name: CreateCampaign :one
INSERT INTO campaigns (tenant_id, play_id, name, status, gmail_account_id, sequence, linkedin_sequence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = $1 AND tenant_id = $2;

-- name: GetCampaignByID :one
SELECT * FROM campaigns WHERE id = $1;

-- name: ListCampaigns :many
SELECT * FROM campaigns WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdateCampaignStatus :one
UPDATE campaigns SET status = $2 WHERE id = $1 RETURNING *;

-- name: UpdateCampaignStats :exec
UPDATE campaigns SET stats = $2 WHERE id = $1;

-- name: UpdateCampaignSequence :exec
UPDATE campaigns SET sequence = $2 WHERE id = $1;

-- Metrics aggregations for the per-campaign dashboard (issue #9).
-- All filter by campaign_id; the handler verifies the campaign belongs
-- to the caller's tenant via GetCampaign before running these. The
-- queries are intentionally five small SELECTs rather than one large
-- joined view so each can be unit-tested in isolation and the SQL
-- planner has the simplest possible shape for each count.

-- name: CountCampaignLeadsTotal :one
SELECT COUNT(*)::bigint
FROM campaign_leads
WHERE campaign_id = $1;

-- name: CountCampaignSentTotal :one
SELECT COUNT(*)::bigint
FROM email_events ee
JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
WHERE cl.campaign_id = $1
  AND ee.event_type = 'sent';

-- CountCampaignSentByStep returns one row per step that has at least
-- one 'sent' event. The handler turns this into an array of
-- {step_order, count} objects for the dashboard's per-step table.
-- name: CountCampaignSentByStep :many
SELECT ee.step::int AS step_order, COUNT(*)::bigint AS count
FROM email_events ee
JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
WHERE cl.campaign_id = $1
  AND ee.event_type = 'sent'
GROUP BY ee.step
ORDER BY ee.step;

-- name: CountCampaignRepliedTotal :one
SELECT COUNT(*)::bigint
FROM email_events ee
JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
WHERE cl.campaign_id = $1
  AND ee.event_type = 'replied';

-- CountCampaignBouncedTotal de-duplicates per campaign_lead: a lead
-- with 2 soft bounces + 1 hard bounce counts as 1. The conceptual
-- contract is "leads that have bounced at least once in this
-- campaign," not "total bounce events" — the dashboard's audience
-- cares about how many addresses are unreachable, not how many DSNs
-- arrived. Event names match issue #2's vocabulary ('bounced' for
-- hard, 'soft-bounce' for soft) rather than the PRD's draft text.
-- name: CountCampaignBouncedTotal :one
SELECT COUNT(DISTINCT ee.campaign_lead_id)::bigint
FROM email_events ee
JOIN campaign_leads cl ON cl.id = ee.campaign_lead_id
WHERE cl.campaign_id = $1
  AND ee.event_type IN ('bounced', 'soft-bounce');

-- CountCampaignUnsubscribedTotal joins each campaign_lead in this
-- campaign to its resolved email (canonical first, legacy fallback)
-- and checks whether that email appears in `unsubscribes` for the
-- campaign's tenant or globally. COUNT(DISTINCT cl.id) collapses the
-- case where a single lead matches both a tenant-scoped and a global
-- unsubscribe row.
-- name: CountCampaignUnsubscribedTotal :one
WITH best AS (
    SELECT DISTINCT ON (em.person_id) em.person_id, em.email
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
)
SELECT COUNT(DISTINCT cl.id)::bigint
FROM campaign_leads cl
JOIN campaigns c       ON c.id = cl.campaign_id
LEFT JOIN persons p    ON p.id = cl.person_id
LEFT JOIN best b       ON b.person_id = p.id
LEFT JOIN leads l      ON l.id = cl.lead_id
JOIN unsubscribes u    ON (u.tenant_id = c.tenant_id OR u.tenant_id IS NULL)
                      AND lower(u.email) = lower(COALESCE(b.email, l.email))
WHERE cl.campaign_id = $1;
