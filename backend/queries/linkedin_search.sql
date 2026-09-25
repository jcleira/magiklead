-- Lookup query for the LinkedIn prospect source. The write-through
-- itself reuses the canonical upsert queries in pdl.sql (organizations,
-- persons, employments) minus the email upsert — the LinkedIn path never
-- writes an emails row — and dedups on the existing FindPersonByIdentifier
-- (ingest.sql) keyed on ('linkedin_url', <profile url>). This file adds
-- only the result-attachment query the search handler needs.

-- ListLinkedInURLsForPersons attaches the linkedin_url identifier to
-- lead-search results so the UI can render a profile link. One row per
-- person in the input set that carries a linkedin_url identifier.
-- name: ListLinkedInURLsForPersons :many
SELECT DISTINCT ON (pi.person_id)
    pi.person_id,
    pi.identifier_value AS linkedin_url
FROM person_identifiers pi
WHERE pi.identifier_type = 'linkedin_url'
  AND pi.person_id = ANY(sqlc.arg('person_ids')::uuid[])
ORDER BY pi.person_id;

-- CreateLinkedInSearch logs one people search run through a connected
-- LinkedIn account and how many profiles it returned (the daily budget
-- counts them all, including results without a public profile).
-- name: CreateLinkedInSearch :exec
INSERT INTO linkedin_searches (tenant_id, linkedin_account_id, filters, result_count)
VALUES ($1, $2, $3, $4);

-- SumLinkedInSearchProfilesSince returns how many profiles the account's
-- searches returned after `since` — the daily budget check.
-- name: SumLinkedInSearchProfilesSince :one
SELECT COALESCE(SUM(result_count), 0)::bigint AS profiles
FROM linkedin_searches
WHERE linkedin_account_id = sqlc.arg(account_id)
  AND created_at > sqlc.arg(since);

-- FindCurrentOrganizationByName returns the organization of the
-- person's current job whose name matches (case-insensitive). LinkedIn
-- results carry no company domain, so a profile seen again reuses the
-- company it was stored with instead of a second organization and a
-- second current job.
-- name: FindCurrentOrganizationByName :one
SELECT o.*
FROM employments e
JOIN organizations o ON o.id = e.organization_id
WHERE e.person_id = sqlc.arg(person_id)
  AND e.is_current = TRUE
  AND lower(o.canonical_name) = lower(sqlc.arg(name)::text)
LIMIT 1;

-- LinkOrganizationIdentifierIfAbsent keys an organization unless the
-- value is already stored (identifier_type + identifier_value is
-- UNIQUE). Returns the rows inserted (0 or 1).
-- name: LinkOrganizationIdentifierIfAbsent :execrows
INSERT INTO organization_identifiers (organization_id, identifier_type, identifier_value, is_primary)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;
