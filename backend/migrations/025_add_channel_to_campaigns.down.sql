DROP INDEX IF EXISTS idx_campaigns_channel;
ALTER TABLE campaigns DROP COLUMN IF EXISTS channel;
