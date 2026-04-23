DROP INDEX IF EXISTS idx_persons_normalized_name_trgm;
ALTER TABLE persons DROP COLUMN IF EXISTS normalized_name;
