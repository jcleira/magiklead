-- name: AddLeadToCampaign :exec
INSERT INTO campaign_leads (campaign_id, lead_id, status)
VALUES ($1, $2, 'queued')
ON CONFLICT (campaign_id, lead_id) DO NOTHING;

-- name: GetDueLeads :many
SELECT cl.id, cl.campaign_id, cl.lead_id, cl.status, cl.current_step, cl.next_send_at,
       l.email, l.first_name, l.last_name, l.company, l.title
FROM campaign_leads cl
JOIN leads l ON cl.lead_id = l.id
WHERE cl.next_send_at <= NOW()
AND cl.status = 'active'
ORDER BY cl.next_send_at
LIMIT $1;

-- name: UpdateCampaignLeadStep :exec
UPDATE campaign_leads
SET current_step = $2, next_send_at = $3, last_sent_at = NOW(), status = $4
WHERE id = $1;

-- name: ListCampaignLeads :many
SELECT cl.*, l.first_name, l.last_name, l.email, l.title, l.company, l.linkedin_url
FROM campaign_leads cl
JOIN leads l ON cl.lead_id = l.id
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
