-- tenant_leads is the per-tenant association with canonical persons.
-- It replaces the old `leads` table over time: canonical persons live
-- in `persons` and are shared across tenants; tenant_leads holds the
-- per-tenant state (who saved this person, status, notes).
--
-- See research.md §"Tenant data" and plan.md §T13.

CREATE TABLE tenant_leads (
    tenant_id        UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    person_id        UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'new',
    notes            TEXT,
    added_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    added_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (tenant_id, person_id)
);

CREATE INDEX idx_tenant_leads_person_id ON tenant_leads(person_id);
CREATE INDEX idx_tenant_leads_status ON tenant_leads(tenant_id, status);
CREATE INDEX idx_tenant_leads_added_at ON tenant_leads(tenant_id, added_at DESC);

-- Bridge campaign_leads to canonical persons. During the transition
-- period campaign_leads holds BOTH lead_id (old) and person_id (new).
-- Once all handlers read from tenant_leads, a follow-up migration can
-- drop lead_id entirely.
ALTER TABLE campaign_leads ADD COLUMN person_id UUID REFERENCES persons(id) ON DELETE CASCADE;
CREATE INDEX idx_campaign_leads_person_id ON campaign_leads(person_id);

-- Backfill tenant_leads from existing campaign_leads. Leads not yet
-- mapped to a canonical person (leads.person_id IS NULL) are skipped —
-- a separate canonicalizer job will pick them up; see plan.md §T01's
-- "backfill job later" note.
INSERT INTO tenant_leads (tenant_id, person_id, status, added_at)
SELECT DISTINCT
    c.tenant_id,
    l.person_id,
    'new',
    cl.created_at
FROM campaign_leads cl
JOIN campaigns c ON c.id = cl.campaign_id
JOIN leads     l ON l.id = cl.lead_id
WHERE l.person_id IS NOT NULL
ON CONFLICT (tenant_id, person_id) DO NOTHING;

-- Backfill campaign_leads.person_id from leads.person_id for any rows
-- whose leads were canonicalized (via T01's nullable person_id column).
UPDATE campaign_leads cl
SET person_id = l.person_id
FROM leads l
WHERE l.id = cl.lead_id
  AND l.person_id IS NOT NULL
  AND cl.person_id IS NULL;
