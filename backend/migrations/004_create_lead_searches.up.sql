CREATE TABLE lead_searches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    query_hash TEXT NOT NULL,
    filters JSONB NOT NULL,
    result_lead_ids UUID[] NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL,
    ttl_days INT DEFAULT 30,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_lead_searches_hash ON lead_searches(query_hash);
