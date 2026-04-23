-- Reverses 013_canonical_schema.up.sql.
-- Drop order matters: dependent tables before their parents. pg_trgm is left
-- in place since dropping a shared extension can break unrelated code.

DROP INDEX IF EXISTS idx_leads_person_id;
ALTER TABLE leads DROP COLUMN IF EXISTS person_id;

DROP TABLE IF EXISTS merge_history;
DROP TABLE IF EXISTS conflict_queue;
DROP TABLE IF EXISTS deletion_blocklist;
DROP TABLE IF EXISTS evidence;

DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS social_profiles;
DROP TABLE IF EXISTS phones;
DROP TABLE IF EXISTS emails;

DROP TABLE IF EXISTS employments;

DROP TABLE IF EXISTS person_identifiers;
DROP TABLE IF EXISTS person_aliases;
DROP TABLE IF EXISTS persons;

DROP TABLE IF EXISTS organization_identifiers;
DROP TABLE IF EXISTS organization_aliases;
DROP TABLE IF EXISTS organizations;

DROP TABLE IF EXISTS source_records;
DROP TABLE IF EXISTS raw_ingests;
DROP TABLE IF EXISTS sources;
