CREATE TABLE campaign_leads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID NOT NULL REFERENCES campaigns(id),
    lead_id UUID NOT NULL REFERENCES leads(id),
    status TEXT DEFAULT 'queued',
    current_step INT DEFAULT 0,
    next_send_at TIMESTAMPTZ,
    last_sent_at TIMESTAMPTZ,
    last_opened_at TIMESTAMPTZ,
    last_replied_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(campaign_id, lead_id)
);

CREATE INDEX idx_campaign_leads_campaign_id ON campaign_leads(campaign_id);
CREATE INDEX idx_campaign_leads_lead_id ON campaign_leads(lead_id);
CREATE INDEX idx_campaign_leads_next_send_at ON campaign_leads(next_send_at) WHERE status = 'active';
