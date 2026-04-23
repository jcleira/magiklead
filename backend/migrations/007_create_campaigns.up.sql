CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    play_id UUID NOT NULL REFERENCES plays(id),
    name TEXT NOT NULL,
    status TEXT DEFAULT 'draft',
    gmail_account_id UUID REFERENCES gmail_accounts(id),
    sequence JSONB NOT NULL DEFAULT '[]',
    linkedin_sequence JSONB,
    stats JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_campaigns_tenant_id ON campaigns(tenant_id);
CREATE INDEX idx_campaigns_status ON campaigns(status);
