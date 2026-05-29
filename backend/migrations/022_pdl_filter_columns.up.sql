-- PDL canonical write-through requires three filter dimensions that
-- the search handler today rejects with 501: industries, company size,
-- and locations. Add the columns onto the canonical so PDL results
-- have somewhere to land and subsequent canonical-only lookups can
-- filter on the same data.

ALTER TABLE organizations
    ADD COLUMN industries TEXT[],
    ADD COLUMN size_range TEXT;

ALTER TABLE persons
    ADD COLUMN location TEXT;

-- updated_at on emails lets the 90-day staleness check fall through
-- to PDL on stale rows even when persons/orgs were touched recently.
ALTER TABLE emails
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX idx_organizations_industries
    ON organizations USING GIN (industries);
CREATE INDEX idx_organizations_size_range
    ON organizations(size_range);
CREATE INDEX idx_persons_location_trgm
    ON persons USING GIN (location gin_trgm_ops);
