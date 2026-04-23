CREATE TABLE email_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    user_id UUID NOT NULL REFERENCES users(id),
    provider TEXT NOT NULL DEFAULT 'smtp',  -- 'smtp' or 'gmail'
    email TEXT NOT NULL,

    -- SMTP fields
    smtp_host TEXT,
    smtp_port INT DEFAULT 587,
    smtp_username TEXT,
    smtp_password TEXT,

    -- Gmail OAuth fields
    access_token TEXT,
    refresh_token TEXT,
    token_expiry TIMESTAMPTZ,

    -- Shared
    sender_name TEXT,
    daily_sent_count INT DEFAULT 0,
    daily_sent_reset_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
