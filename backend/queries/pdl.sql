-- Write-through queries for the People Data Labs integration. PDL
-- search results land in the canonical graph (persons, organizations,
-- employments, emails) so subsequent identical lookups across any
-- tenant hit the cache for free. Stale rows (>90 days) re-trigger a
-- PDL call from the handler layer.

-- FindOrganizationByPrimaryDomain returns the most-recently-updated
-- organization for a given domain. PDL identifies companies by domain
-- as the stable join key.
-- name: FindOrganizationByPrimaryDomain :one
SELECT * FROM organizations
WHERE primary_domain = $1
ORDER BY updated_at DESC NULLS LAST, created_at DESC
LIMIT 1;

-- name: CreateOrganizationFromPDL :one
INSERT INTO organizations (canonical_name, primary_domain, industries, size_range, updated_at)
VALUES ($1, $2, $3, $4, NOW())
RETURNING *;

-- name: UpdateOrganizationFromPDL :one
UPDATE organizations
SET canonical_name = $2,
    industries     = COALESCE($3, industries),
    size_range     = COALESCE($4, size_range),
    updated_at     = NOW()
WHERE id = $1
RETURNING *;

-- FindPersonByPDLID looks up a canonical person by the
-- person_identifiers row keyed on PDL's stable person ID.
-- name: FindPersonByPDLID :one
SELECT p.* FROM persons p
JOIN person_identifiers pi ON pi.person_id = p.id
WHERE pi.identifier_type = 'pdl_id' AND pi.identifier_value = $1
LIMIT 1;

-- name: CreatePersonFromPDL :one
INSERT INTO persons (canonical_name, first_name, last_name, normalized_name, location, has_email, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: UpdatePersonFromPDL :one
-- has_email is sticky: once a person is known emailable we never flip
-- it back (a later write that happens to lack the field shouldn't lose
-- the signal), so OR the existing value with the incoming one.
UPDATE persons
SET canonical_name = $2,
    first_name      = COALESCE($3, first_name),
    last_name       = COALESCE($4, last_name),
    normalized_name = $5,
    location        = COALESCE($6, location),
    has_email       = persons.has_email OR $7,
    updated_at      = NOW()
WHERE id = $1
RETURNING *;

-- FindCurrentEmployment returns the active employment (is_current=TRUE)
-- between a person and an organization, regardless of title. Distinct
-- from FindCurrentEmploymentByTitle in verification.sql which keys on
-- title too.
-- name: FindCurrentEmployment :one
SELECT * FROM employments
WHERE person_id = $1 AND organization_id = $2 AND is_current = TRUE
LIMIT 1;

-- name: UpdateEmploymentTitle :exec
UPDATE employments SET title = $2 WHERE id = $1;

-- UpsertEmailVerifiedFromPDL writes a PDL-verified email through to
-- the canonical. ON CONFLICT preserves any prior person_id linkage
-- (PDL's source person may differ from the canonical's, but the
-- email-to-person association is stronger than transient PDL state).
-- name: UpsertEmailVerifiedFromPDL :one
INSERT INTO emails (email, person_id, verified_at, verification_method, updated_at)
VALUES (LOWER($1), $2, NOW(), 'pdl-verified', NOW())
ON CONFLICT (email) DO UPDATE
    SET person_id           = COALESCE(emails.person_id, EXCLUDED.person_id),
        verified_at         = NOW(),
        verification_method = 'pdl-verified',
        updated_at          = NOW()
RETURNING *;
