DROP INDEX IF EXISTS idx_persons_location_trgm;
DROP INDEX IF EXISTS idx_organizations_size_range;
DROP INDEX IF EXISTS idx_organizations_industries;

ALTER TABLE emails DROP COLUMN IF EXISTS updated_at;
ALTER TABLE persons DROP COLUMN IF EXISTS location;
ALTER TABLE organizations
    DROP COLUMN IF EXISTS size_range,
    DROP COLUMN IF EXISTS industries;
