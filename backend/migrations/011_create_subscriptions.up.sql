CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    plan TEXT NOT NULL DEFAULT 'free',
    leads_used INT DEFAULT 0,
    leads_limit INT NOT NULL DEFAULT 100,
    sequences_used INT DEFAULT 0,
    sequences_limit INT NOT NULL DEFAULT 300,
    current_period_start TIMESTAMPTZ,
    current_period_end TIMESTAMPTZ,
    stripe_customer_id TEXT,
    stripe_subscription_id TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_subscriptions_tenant_id ON subscriptions(tenant_id);
