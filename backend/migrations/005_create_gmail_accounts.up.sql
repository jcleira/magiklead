CREATE TABLE gmail_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID NOT NULL REFERENCES users(id),
    email TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry TIMESTAMPTZ,
    daily_sent_count INT DEFAULT 0,
    daily_sent_reset_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
