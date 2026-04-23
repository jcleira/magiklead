DROP INDEX IF EXISTS idx_campaign_leads_person_id;
ALTER TABLE campaign_leads DROP COLUMN IF EXISTS person_id;

DROP INDEX IF EXISTS idx_tenant_leads_added_at;
DROP INDEX IF EXISTS idx_tenant_leads_status;
DROP INDEX IF EXISTS idx_tenant_leads_person_id;
DROP TABLE IF EXISTS tenant_leads;
