CREATE TABLE linkedin_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- ON DELETE CASCADE so account erasure (the GDPR delete in
    -- internal/handler/account.go, which DELETEs the tenant row last)
    -- removes a tenant's LinkedIn connections automatically — that
    -- handler predates this table and deletes its known tenant tables
    -- explicitly, so without the cascade a connected account would
    -- FK-block the tenant delete.
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    -- Unipile owns the session; we hold no OAuth tokens, only the
    -- Unipile-side account id. Globally unique so a replayed
    -- account.connected webhook upserts one row (idempotent ingest).
    unipile_account_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'connecting'
        CHECK (status IN ('connecting', 'active', 'warming', 'restricted', 'disconnected')),
    warmup_started_at TIMESTAMPTZ,
    weekly_invite_count INT NOT NULL DEFAULT 0,
    weekly_window_started_at TIMESTAMPTZ,
    daily_invite_count INT NOT NULL DEFAULT 0,
    daily_reset_at TIMESTAMPTZ,
    acceptance_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_linkedin_accounts_tenant ON linkedin_accounts (tenant_id);
