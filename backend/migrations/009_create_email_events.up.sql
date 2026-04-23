CREATE TABLE email_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_lead_id UUID NOT NULL REFERENCES campaign_leads(id),
    event_type TEXT NOT NULL,
    step INT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_email_events_campaign_lead_id ON email_events(campaign_lead_id);
