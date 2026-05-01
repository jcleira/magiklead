-- Legacy: add a row via the lead_id path. Kept while the pipeline
-- still runs; T07 deletes it.
-- name: AddLeadToCampaign :exec
INSERT INTO campaign_leads (campaign_id, lead_id, status)
VALUES ($1, $2, 'queued')
ON CONFLICT DO NOTHING;

-- New canonical path — add a saved person to a campaign.
-- name: AddPersonToCampaign :exec
INSERT INTO campaign_leads (campaign_id, person_id, status)
VALUES ($1, $2, 'queued')
ON CONFLICT DO NOTHING;

-- GetDueLeads resolves the contact fields from either the canonical
-- persons graph (when campaign_leads.person_id is set) or the legacy
-- leads row (lead_id). The sender reads a unified row shape regardless
-- of which path populated it.
--
-- For the canonical path we pull the best email per person (verified
-- wins, then fewest bounces, then oldest) and the current employment
-- for title/company.
-- name: GetDueLeads :many
WITH best AS (
    SELECT DISTINCT ON (em.person_id)
        em.person_id,
        em.email,
        em.verified_at,
        em.is_catchall,
        em.bounce_count
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
), current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.lead_id,
    cl.person_id,
    cl.status,
    cl.current_step,
    cl.next_send_at,
    COALESCE(b.email, l.email, '')::text             AS email,
    COALESCE(p.first_name, l.first_name, '')::text   AS first_name,
    COALESCE(p.last_name,  l.last_name,  '')::text   AS last_name,
    COALESCE(o.canonical_name, l.company, '')::text  AS company,
    COALESCE(ce.title, l.title)                      AS title
FROM campaign_leads cl
LEFT JOIN leads         l  ON l.id  = cl.lead_id
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN best          b  ON b.person_id = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
WHERE cl.next_send_at <= NOW()
  AND cl.status = 'active'
ORDER BY cl.next_send_at
LIMIT $1;

-- name: UpdateCampaignLeadStep :exec
UPDATE campaign_leads
SET current_step = $2, next_send_at = $3, last_sent_at = NOW(), status = $4
WHERE id = $1;

-- ListCampaignLeads returns the rows for one campaign with contact
-- fields resolved from either the canonical persons graph (preferred)
-- or the legacy leads table. Mirrors GetDueLeads's CTE structure so
-- that the UI displays the same unified shape.
-- name: ListCampaignLeads :many
WITH best AS (
    SELECT DISTINCT ON (em.person_id)
        em.person_id,
        em.email
    FROM emails em
    ORDER BY em.person_id,
             (em.verified_at IS NOT NULL) DESC,
             em.bounce_count ASC,
             em.created_at ASC
), current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
)
SELECT
    cl.id,
    cl.campaign_id,
    cl.lead_id,
    cl.person_id,
    cl.status,
    cl.current_step,
    cl.next_send_at,
    cl.last_sent_at,
    cl.last_opened_at,
    cl.last_replied_at,
    cl.created_at,
    COALESCE(p.first_name,    l.first_name, '')::text   AS first_name,
    COALESCE(p.last_name,     l.last_name,  '')::text   AS last_name,
    COALESCE(b.email,         l.email,      '')::text   AS email,
    COALESCE(ce.title,        l.title)                  AS title,
    COALESCE(o.canonical_name, l.company,   '')::text   AS company,
    l.linkedin_url                                       AS linkedin_url
FROM campaign_leads cl
LEFT JOIN leads         l  ON l.id  = cl.lead_id
LEFT JOIN persons       p  ON p.id  = cl.person_id
LEFT JOIN best          b  ON b.person_id = cl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = cl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
WHERE cl.campaign_id = $1
ORDER BY cl.created_at DESC;

-- name: CountCampaignLeadsByStatus :many
SELECT status, COUNT(*) as count FROM campaign_leads
WHERE campaign_id = $1
GROUP BY status;

-- name: ActivateCampaignLeads :exec
UPDATE campaign_leads SET status = 'active', next_send_at = NOW()
WHERE campaign_id = $1 AND status = 'queued';

-- name: PauseCampaignLeads :exec
UPDATE campaign_leads SET next_send_at = NULL
WHERE campaign_id = $1 AND status = 'active';
