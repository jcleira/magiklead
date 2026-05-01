-- T06: campaign_leads cutover from legacy lead_id to canonical person_id.
-- Rows may now specify person_id (new path) or lead_id (legacy), at
-- least one is required. Dropping the NOT NULL on lead_id lets new
-- campaigns write person_id without carrying a synthetic legacy row.
--
-- The original UNIQUE(campaign_id, lead_id) constraint is replaced by
-- two partial unique indexes so the same person/lead can't be added
-- to a campaign twice, but NULLs don't collide.

ALTER TABLE campaign_leads
    ALTER COLUMN lead_id DROP NOT NULL;

ALTER TABLE campaign_leads
    DROP CONSTRAINT IF EXISTS campaign_leads_campaign_id_lead_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_campaign_leads_campaign_lead
    ON campaign_leads (campaign_id, lead_id)
    WHERE lead_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_campaign_leads_campaign_person
    ON campaign_leads (campaign_id, person_id)
    WHERE person_id IS NOT NULL;

ALTER TABLE campaign_leads
    DROP CONSTRAINT IF EXISTS campaign_leads_any_target;
ALTER TABLE campaign_leads
    ADD CONSTRAINT campaign_leads_any_target
    CHECK (lead_id IS NOT NULL OR person_id IS NOT NULL);
