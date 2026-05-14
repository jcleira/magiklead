-- Issue #2: store the Gmail send-side message-id on the email_events
-- row so the History API poller (issues #4/#5) can match incoming
-- replies and bounces back to the originating campaign_lead.
--
-- Column is nullable because every existing row predates this column
-- and SMTP sends (the legacy path) do not produce a Gmail message-id.

ALTER TABLE email_events
    ADD COLUMN IF NOT EXISTS gmail_message_id TEXT;

CREATE INDEX IF NOT EXISTS idx_email_events_gmail_message_id
    ON email_events(gmail_message_id)
    WHERE gmail_message_id IS NOT NULL;
