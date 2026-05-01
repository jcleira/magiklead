-- Reverses 017: restore NOT NULL on lead_id and the original unique
-- constraint. Rows with NULL lead_id must be deleted first — this
-- down-migration will error if any person-only rows exist.

ALTER TABLE campaign_leads
    DROP CONSTRAINT IF EXISTS campaign_leads_any_target;

DROP INDEX IF EXISTS uniq_campaign_leads_campaign_person;
DROP INDEX IF EXISTS uniq_campaign_leads_campaign_lead;

ALTER TABLE campaign_leads
    ALTER COLUMN lead_id SET NOT NULL;

ALTER TABLE campaign_leads
    ADD CONSTRAINT campaign_leads_campaign_id_lead_id_key
    UNIQUE (campaign_id, lead_id);
