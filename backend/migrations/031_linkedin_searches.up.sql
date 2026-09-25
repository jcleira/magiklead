-- linkedin_searches logs every people search the app runs through a
-- tenant's connected LinkedIn account (Sales Navigator, via Unipile).
-- The search runs AS that account, and LinkedIn watches the volume, so
-- the app keeps a daily profile budget per account: the search handler
-- sums result_count over the last 24 hours before each call. Unipile's
-- guidance is at most 1,000 profiles per day on a standard account; the
-- handler stays far below that.
CREATE TABLE linkedin_searches (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    linkedin_account_id UUID NOT NULL REFERENCES linkedin_accounts(id) ON DELETE CASCADE,
    filters             JSONB NOT NULL,
    result_count        INTEGER NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_linkedin_searches_account_created
    ON linkedin_searches (linkedin_account_id, created_at);
