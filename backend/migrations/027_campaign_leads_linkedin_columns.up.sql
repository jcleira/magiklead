-- The LinkedIn send tick (issue #4) needs to bind a campaign_lead to the
-- account that sent its invite and to the Unipile-side invitation, then
-- park it waiting for acceptance. `accepted_at` is set by issue #5 when
-- the acceptance webhook/poll fires. linkedin_account_id uses ON DELETE
-- SET NULL: disconnecting an account (issue #8) must not delete the lead
-- rows that reference it — they stay for forensics, just unbound.
--
-- No status enum to alter: campaign_leads.status is a free TEXT column
-- (migration 008), so the new 'awaiting_accept' / 'not_accepted' values
-- the LinkedIn engine writes need no schema change.
ALTER TABLE campaign_leads
    ADD COLUMN linkedin_account_id UUID REFERENCES linkedin_accounts(id) ON DELETE SET NULL,
    ADD COLUMN linkedin_invitation_id TEXT,
    ADD COLUMN linkedin_chat_id TEXT,
    ADD COLUMN accepted_at TIMESTAMPTZ;
