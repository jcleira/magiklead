-- name: GetSourceByName :one
SELECT * FROM sources WHERE name = $1;

-- name: CreateSource :one
INSERT INTO sources (name, type, api_endpoint)
VALUES ($1, $2, $3)
RETURNING *;

-- name: TouchSourceLastIngested :exec
UPDATE sources SET last_ingested_at = NOW() WHERE id = $1;

-- name: GetRawIngestByChecksum :one
SELECT * FROM raw_ingests
WHERE source_id = $1 AND checksum = $2
LIMIT 1;

-- name: CreateRawIngest :one
INSERT INTO raw_ingests (source_id, file_url, checksum, size_bytes, meta)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateSourceRecord :one
INSERT INTO source_records (source_id, raw_ingest_id, external_id, fields)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateEvidence :one
INSERT INTO evidence (canonical_table, canonical_id, source_record_id, confidence, conflict_flag)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreatePerson :one
INSERT INTO persons (canonical_name, first_name, last_name, normalized_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- FindPersonsByNormalizedName does a cross-org fuzzy match — used for
-- T08 when neither CIK nor LinkedIn identified the person and an
-- at-org fuzzy match came up empty.
-- name: FindPersonsByNormalizedName :many
SELECT p.id, similarity(p.normalized_name, sqlc.arg('query_name')::text)::real AS sim
FROM persons p
WHERE p.normalized_name % sqlc.arg('query_name')::text
ORDER BY similarity(p.normalized_name, sqlc.arg('query_name')::text) DESC
LIMIT 5;

-- name: UpdatePersonNormalizedName :exec
UPDATE persons SET normalized_name = $2 WHERE id = $1;

-- name: CreateOrganization :one
INSERT INTO organizations (canonical_name, primary_domain)
VALUES ($1, $2)
RETURNING *;

-- name: CreateEmployment :one
INSERT INTO employments (person_id, organization_id, title, start_date, end_date, is_current)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: FindOrganizationByIdentifier :one
SELECT o.*
FROM organizations o
JOIN organization_identifiers oi ON oi.organization_id = o.id
WHERE oi.identifier_type = $1 AND oi.identifier_value = $2
LIMIT 1;

-- name: CreateOrganizationIdentifier :exec
INSERT INTO organization_identifiers (organization_id, identifier_type, identifier_value, is_primary)
VALUES ($1, $2, $3, $4);

-- name: CreateOrganizationAlias :exec
INSERT INTO organization_aliases (organization_id, alias, alias_type)
VALUES ($1, $2, $3)
ON CONFLICT ON CONSTRAINT organization_aliases_unique DO NOTHING;

-- name: FindPersonByIdentifier :one
SELECT p.*
FROM persons p
JOIN person_identifiers pi ON pi.person_id = p.id
WHERE pi.identifier_type = $1 AND pi.identifier_value = $2
LIMIT 1;

-- name: CreatePersonIdentifier :exec
INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary)
VALUES ($1, $2, $3, $4);

-- GetPersonIdentifier reads a single identifier value of a given type off a
-- person — the read side of the member-id cache (issue #5): given a person
-- and 'linkedin_member_id', it returns the cached Unipile member id, or
-- pgx.ErrNoRows on a miss (the signal to resolve upstream and persist).
-- name: GetPersonIdentifier :one
SELECT identifier_value
FROM person_identifiers
WHERE person_id = $1 AND identifier_type = $2
LIMIT 1;

-- name: CreatePersonAlias :exec
INSERT INTO person_aliases (person_id, alias, alias_type)
VALUES ($1, $2, $3)
ON CONFLICT ON CONSTRAINT person_aliases_unique DO NOTHING;

-- FindPersonsByNameAtOrg returns persons with fuzzy-matching canonical
-- names who have at least one employment at the given organization,
-- sorted by similarity. Uses the GIN trgm index via `%`.
-- name: FindPersonsByNameAtOrg :many
SELECT p.id, similarity(p.canonical_name, sqlc.arg('query_name')::text)::real AS sim
FROM persons p
WHERE p.canonical_name % sqlc.arg('query_name')::text
  AND EXISTS (
    SELECT 1 FROM employments e
    WHERE e.person_id = p.id AND e.organization_id = sqlc.arg('organization_id')
  )
ORDER BY similarity(p.canonical_name, sqlc.arg('query_name')::text) DESC
LIMIT 5;

-- name: FindCurrentEmploymentByTitle :one
SELECT * FROM employments
WHERE person_id = $1
  AND organization_id = $2
  AND is_current = TRUE
  AND title IS NOT DISTINCT FROM $3
LIMIT 1;

-- name: CreateConflictQueueItem :one
INSERT INTO conflict_queue (canonical_table, canonical_id, new_source_record_id, status)
VALUES ($1, $2, $3, $4)
RETURNING *;
