CREATE TABLE plays (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    description TEXT,
    icp JSONB NOT NULL,
    search_query JSONB NOT NULL,
    channels TEXT[] NOT NULL,
    status TEXT DEFAULT 'draft',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_plays_tenant_id ON plays(tenant_id);
