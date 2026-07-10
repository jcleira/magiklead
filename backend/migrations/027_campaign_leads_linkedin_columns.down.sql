ALTER TABLE campaign_leads
    DROP COLUMN IF EXISTS linkedin_account_id,
    DROP COLUMN IF EXISTS linkedin_invitation_id,
    DROP COLUMN IF EXISTS linkedin_chat_id,
    DROP COLUMN IF EXISTS accepted_at;
