-- Privacy / right-to-erasure queries (plan §T16).
--
-- These power POST /api/v1/privacy/erasure: identify the canonical
-- person(s) matching the requester's identifiers, hard-delete them
-- (CASCADE clears employments / emails / aliases / etc.), and write
-- hashed identifier rows to deletion_blocklist so the resolver can
-- refuse to re-create the record from a future ingest.

-- FindPersonIDsByEmail returns person IDs for any verified or
-- unverified email matching the input. Email matching is
-- case-insensitive (LOWER on both sides). NULL person_id rows are
-- excluded — those represent unattached emails.
-- name: FindPersonIDsByEmail :many
SELECT DISTINCT person_id FROM emails
WHERE LOWER(email) = LOWER($1)
  AND person_id IS NOT NULL;

-- FindPersonIDsByLinkedIn returns person IDs whose person_identifiers
-- carries the given normalized LinkedIn URL. Caller is responsible
-- for normalization (see ingest/resolver.go normalizeLinkedInURL).
-- name: FindPersonIDsByLinkedIn :many
SELECT DISTINCT person_id FROM person_identifiers
WHERE identifier_type = 'linkedin' AND identifier_value = $1;

-- FindPersonIDsByNormalizedNameAndOrg matches the person's normalized
-- name (alphabetically-sorted lowercase tokens, see resolver.go
-- normalizeName) against an org's canonical_name or any alias. ILIKE
-- with bidirectional substring lets "Apple" match "Apple Inc" without
-- requiring a normalized_canonical_name column on organizations.
-- name: FindPersonIDsByNormalizedNameAndOrg :many
SELECT DISTINCT p.id FROM persons p
JOIN employments e ON e.person_id = p.id
JOIN organizations o ON o.id = e.organization_id
WHERE p.normalized_name = sqlc.arg('normalized_name')::text
  AND (
    o.canonical_name ILIKE '%' || sqlc.arg('company')::text || '%'
    OR sqlc.arg('company')::text ILIKE '%' || o.canonical_name || '%'
    OR EXISTS (
        SELECT 1 FROM organization_aliases oa
        WHERE oa.organization_id = o.id
          AND (oa.alias ILIKE '%' || sqlc.arg('company')::text || '%'
               OR sqlc.arg('company')::text ILIKE '%' || oa.alias || '%')
    )
  );

-- AnyHashBlocked is the resolver's gate before creating a new
-- canonical person — pass every identifier hash the source record
-- yields, get back a single bool. Hashes are SHA-256 hex; collisions
-- are negligible so we don't filter by identifier_type here.
-- name: AnyHashBlocked :one
SELECT EXISTS(
    SELECT 1 FROM deletion_blocklist
    WHERE identifier_hash = ANY(sqlc.arg('hashes')::text[])
) AS blocked;

-- InsertBlocklistEntry is idempotent: re-erasing the same identifier
-- doesn't error.
-- name: InsertBlocklistEntry :exec
INSERT INTO deletion_blocklist (identifier_type, identifier_hash, reason)
VALUES ($1, $2, $3)
ON CONFLICT (identifier_type, identifier_hash) DO NOTHING;
