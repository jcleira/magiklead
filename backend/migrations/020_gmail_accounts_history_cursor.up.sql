-- Issue #2: cursor state for the Gmail History API poller (issues #4
-- and #5). last_history_id is the watermark Gmail returns; persisting
-- it survives worker restarts. last_polled_at lets ops monitor poll
-- liveness without scraping logs.
--
-- Both nullable: pre-existing rows have no cursor, and a freshly-
-- connected mailbox has neither until the first poll completes.

ALTER TABLE gmail_accounts
    ADD COLUMN IF NOT EXISTS last_history_id TEXT;

ALTER TABLE gmail_accounts
    ADD COLUMN IF NOT EXISTS last_polled_at TIMESTAMPTZ;
