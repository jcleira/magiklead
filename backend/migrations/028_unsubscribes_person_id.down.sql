DROP INDEX IF EXISTS idx_unsubscribes_person;
DROP INDEX IF EXISTS uniq_unsubscribes_tenant_person;
ALTER TABLE unsubscribes DROP CONSTRAINT IF EXISTS unsubscribes_email_or_person;
-- Person-only rows have a NULL email; they must go before email can be
-- restored to NOT NULL. Dropping the column would otherwise leave them
-- behind and the SET NOT NULL would fail.
DELETE FROM unsubscribes WHERE email IS NULL;
ALTER TABLE unsubscribes DROP COLUMN IF EXISTS person_id;
ALTER TABLE unsubscribes ALTER COLUMN email SET NOT NULL;
