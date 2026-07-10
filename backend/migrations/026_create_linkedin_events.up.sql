-- linkedin_events is the LinkedIn rail's analogue of email_events: one
-- row per action the LinkedIn engine takes against a campaign_lead
-- (invite_sent, failed, and — in later slices — accepted/dm_sent/replied).
-- Mirrors email_events (009) plus the gmail_message_id idea (019), but
-- carries the two Unipile identifiers the downstream pollers need to
-- match an invite/DM back to its campaign_lead: the invitation id and
-- the chat id (the chat only exists once the invite is accepted, so it
-- is nullable and stays NULL on invite_sent).
CREATE TABLE linkedin_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_lead_id UUID NOT NULL REFERENCES campaign_leads(id),
    event_type TEXT NOT NULL,
    step INT NOT NULL,
    metadata JSONB,
    unipile_message_id TEXT,
    unipile_chat_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_linkedin_events_campaign_lead_id ON linkedin_events(campaign_lead_id);
