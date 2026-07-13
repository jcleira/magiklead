-- Route a campaign to the email rail or the LinkedIn rail. Existing
-- campaigns predate LinkedIn and are all email, so the column defaults
-- to 'email' and backfills every existing row to 'email' implicitly via
-- the DEFAULT. The LinkedIn tick (issue #4) reads channel='linkedin'.
ALTER TABLE campaigns
    ADD COLUMN channel TEXT NOT NULL DEFAULT 'email';

CREATE INDEX idx_campaigns_channel ON campaigns(channel);
