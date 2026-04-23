-- Canonical lead database schema.
-- Implements the "canonical model, multi-source provenance" design from
-- docs/2026-04-21-lead-database-architecture/research.md.

-- pg_trgm powers fuzzy name/title matching used by entity resolution and
-- search (sub-second queries target in research.md §4.2).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================================
-- Sources + raw provenance
-- ============================================================================

CREATE TABLE sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT UNIQUE NOT NULL,
    type TEXT NOT NULL,
    api_endpoint TEXT,
    last_ingested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE raw_ingests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id),
    file_url TEXT NOT NULL,
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    checksum TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    meta JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_raw_ingests_source_id ON raw_ingests(source_id);
CREATE INDEX idx_raw_ingests_checksum ON raw_ingests(checksum);
CREATE INDEX idx_raw_ingests_meta ON raw_ingests USING GIN (meta);

CREATE TABLE source_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id UUID NOT NULL REFERENCES sources(id),
    raw_ingest_id UUID NOT NULL REFERENCES raw_ingests(id),
    external_id TEXT,
    fields JSONB NOT NULL DEFAULT '{}',
    ingested_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_source_records_source_id ON source_records(source_id);
CREATE INDEX idx_source_records_raw_ingest_id ON source_records(raw_ingest_id);
CREATE INDEX idx_source_records_external_id ON source_records(source_id, external_id);
CREATE INDEX idx_source_records_fields ON source_records USING GIN (fields);

-- ============================================================================
-- Canonical organizations
-- ============================================================================

CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_name TEXT NOT NULL,
    primary_domain TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_organizations_canonical_name_trgm
    ON organizations USING GIN (canonical_name gin_trgm_ops);
CREATE INDEX idx_organizations_primary_domain_trgm
    ON organizations USING GIN (primary_domain gin_trgm_ops);
CREATE INDEX idx_organizations_primary_domain ON organizations(primary_domain);

CREATE TABLE organization_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    alias_type TEXT NOT NULL
);

CREATE INDEX idx_organization_aliases_org_id ON organization_aliases(organization_id);
CREATE INDEX idx_organization_aliases_alias_trgm
    ON organization_aliases USING GIN (alias gin_trgm_ops);

CREATE TABLE organization_identifiers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier_type TEXT NOT NULL,
    identifier_value TEXT NOT NULL,
    is_primary BOOLEAN DEFAULT FALSE,
    UNIQUE (identifier_type, identifier_value)
);

CREATE INDEX idx_organization_identifiers_org_id ON organization_identifiers(organization_id);

-- ============================================================================
-- Canonical persons
-- ============================================================================

CREATE TABLE persons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_name TEXT NOT NULL,
    first_name TEXT,
    last_name TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_persons_canonical_name_trgm
    ON persons USING GIN (canonical_name gin_trgm_ops);
CREATE INDEX idx_persons_first_name_trgm
    ON persons USING GIN (first_name gin_trgm_ops);
CREATE INDEX idx_persons_last_name_trgm
    ON persons USING GIN (last_name gin_trgm_ops);

CREATE TABLE person_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    alias_type TEXT NOT NULL
);

CREATE INDEX idx_person_aliases_person_id ON person_aliases(person_id);
CREATE INDEX idx_person_aliases_alias_trgm
    ON person_aliases USING GIN (alias gin_trgm_ops);

CREATE TABLE person_identifiers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    identifier_type TEXT NOT NULL,
    identifier_value TEXT NOT NULL UNIQUE,
    is_primary BOOLEAN DEFAULT FALSE
);

CREATE INDEX idx_person_identifiers_person_id ON person_identifiers(person_id);
CREATE INDEX idx_person_identifiers_type ON person_identifiers(identifier_type);

-- ============================================================================
-- Employments (person ↔ organization over time)
-- ============================================================================

CREATE TABLE employments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    title TEXT,
    start_date DATE,
    end_date DATE,
    is_current BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_employments_person_id ON employments(person_id);
CREATE INDEX idx_employments_org_id ON employments(organization_id);
CREATE INDEX idx_employments_title_trgm ON employments USING GIN (title gin_trgm_ops);
-- Partial index for "current employment" lookups, the hottest query path.
CREATE INDEX idx_employments_current
    ON employments(person_id) WHERE is_current = TRUE;

-- ============================================================================
-- Contact data (emails / phones / socials / domains)
-- ============================================================================

CREATE TABLE emails (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    person_id UUID REFERENCES persons(id) ON DELETE CASCADE,
    verified_at TIMESTAMPTZ,
    verification_method TEXT,
    bounce_count INT DEFAULT 0,
    is_catchall BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_emails_person_id ON emails(person_id);

CREATE TABLE phones (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone TEXT NOT NULL,
    person_id UUID REFERENCES persons(id) ON DELETE CASCADE,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_phones_person_id ON phones(person_id);
CREATE INDEX idx_phones_phone ON phones(phone);

CREATE TABLE social_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,
    handle TEXT,
    url TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (platform, url)
);

CREATE INDEX idx_social_profiles_person_id ON social_profiles(person_id);
CREATE INDEX idx_social_profiles_platform_handle ON social_profiles(platform, handle);

CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain TEXT NOT NULL UNIQUE,
    mx_records TEXT[],
    is_catchall BOOLEAN,
    last_verified_at TIMESTAMPTZ
);

-- ============================================================================
-- Evidence + operational tables
-- ============================================================================

-- evidence.canonical_id is polymorphic (keyed by canonical_table) so no FK.
CREATE TABLE evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_table TEXT NOT NULL,
    canonical_id UUID NOT NULL,
    source_record_id UUID NOT NULL REFERENCES source_records(id) ON DELETE CASCADE,
    confidence SMALLINT NOT NULL,
    conflict_flag BOOLEAN DEFAULT FALSE,
    resolved_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_evidence_canonical ON evidence(canonical_table, canonical_id);
CREATE INDEX idx_evidence_source_record_id ON evidence(source_record_id);
CREATE INDEX idx_evidence_conflict ON evidence(conflict_flag) WHERE conflict_flag = TRUE;

CREATE TABLE deletion_blocklist (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier_hash TEXT NOT NULL,
    identifier_type TEXT NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (identifier_type, identifier_hash)
);

CREATE INDEX idx_deletion_blocklist_hash ON deletion_blocklist(identifier_hash);

CREATE TABLE conflict_queue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    canonical_table TEXT NOT NULL,
    canonical_id UUID NOT NULL,
    new_source_record_id UUID NOT NULL REFERENCES source_records(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_conflict_queue_status ON conflict_queue(status);
CREATE INDEX idx_conflict_queue_canonical ON conflict_queue(canonical_table, canonical_id);

CREATE TABLE merge_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    surviving_id UUID NOT NULL,
    merged_id UUID NOT NULL,
    canonical_table TEXT NOT NULL,
    merged_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_merge_history_surviving ON merge_history(canonical_table, surviving_id);
CREATE INDEX idx_merge_history_merged ON merge_history(canonical_table, merged_id);

-- ============================================================================
-- Bridge old `leads` table to canonical persons (backfill job lands later).
-- ============================================================================

ALTER TABLE leads ADD COLUMN person_id UUID REFERENCES persons(id);
CREATE INDEX idx_leads_person_id ON leads(person_id);
