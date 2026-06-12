DROP INDEX IF EXISTS idx_email_events_gmail_message_id;
ALTER TABLE email_events DROP COLUMN IF EXISTS gmail_message_id;
