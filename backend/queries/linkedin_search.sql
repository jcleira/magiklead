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
