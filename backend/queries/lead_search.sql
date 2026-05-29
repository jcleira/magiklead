-- SearchPersons returns canonical persons matching the request
-- filters, joined to their current employment and employer, ranked
-- by title similarity (pg_trgm) when titles are provided. The handler
-- runs ListBestEmailsForPersons in a second roundtrip to attach
-- email/verified/catchall fields per result — sqlc 1.28 doesn't
-- infer nullability through LEFT JOIN or scalar subqueries, so
-- splitting the query keeps both sides cleanly typed.
--
-- Per plan §T12 + issue #7 (PDL integration). When industries,
-- company_size, locations, or require_fresh is supplied, the query
-- gates results to canonical rows that match the PDL filter
-- dimensions (industries on organizations, size_range, person
-- location) and to rows whose updated_at falls within the 90-day
-- staleness window. If the result is empty the handler then routes
-- to PDL and re-queries.
--
-- Performance: title fuzzy match rides the GIN trgm index on
-- employments.title; is_current uses the partial index. The email
-- EXISTS check uses idx_emails_person_id. Sub-500ms target on 500K
-- persons per plan §6.

-- name: SearchPersons :many
SELECT
    p.id              AS person_id,
    p.canonical_name  AS person_canonical_name,
    p.first_name,
    p.last_name,
    p.location,
    e.title,
    o.id              AS organization_id,
    o.canonical_name  AS organization_name,
    o.primary_domain,
    o.industries,
    o.size_range,
    COALESCE(
        (SELECT MAX(similarity(e.title, t))
         FROM unnest(COALESCE(sqlc.narg('titles')::text[], ARRAY[]::text[])) AS t),
        0
    )::real           AS title_score
FROM employments e
JOIN persons p
    ON p.id = e.person_id
JOIN organizations o
    ON o.id = e.organization_id
WHERE
    e.is_current = TRUE
    AND (sqlc.narg('titles')::text[] IS NULL OR EXISTS (
        SELECT 1
        FROM unnest(sqlc.narg('titles')::text[]) AS t
        WHERE e.title % t
    ))
    AND (NOT sqlc.arg('with_email')::boolean OR EXISTS (
        SELECT 1 FROM emails em
        WHERE em.person_id = p.id AND em.verified_at IS NOT NULL
    ))
    AND (sqlc.narg('industries')::text[] IS NULL OR o.industries && sqlc.narg('industries')::text[])
    AND (sqlc.narg('company_size')::text IS NULL OR o.size_range = sqlc.narg('company_size')::text)
    AND (sqlc.narg('locations')::text[] IS NULL OR EXISTS (
        SELECT 1
        FROM unnest(sqlc.narg('locations')::text[]) AS loc
        WHERE p.location ILIKE '%' || loc || '%'
    ))
    AND (NOT sqlc.arg('require_fresh')::boolean
         OR p.updated_at > NOW() - INTERVAL '90 days')
ORDER BY title_score DESC NULLS LAST,
         p.canonical_name ASC,
         p.id ASC
LIMIT sqlc.arg('result_limit') OFFSET sqlc.arg('result_offset');

-- ListBestEmailsForPersons returns the highest-priority email per
-- person from the input UUID set: prefer verified, then fewest
-- bounces, then oldest. Only persons with at least one email appear
-- in the result. Used by the search handler to attach contact data.

-- name: ListBestEmailsForPersons :many
SELECT DISTINCT ON (em.person_id)
    em.person_id,
    em.email,
    em.verified_at,
    em.is_catchall,
    em.bounce_count,
    em.verification_method
FROM emails em
WHERE em.person_id = ANY(sqlc.arg('person_ids')::uuid[])
ORDER BY em.person_id,
         (em.verified_at IS NOT NULL) DESC,
         em.bounce_count ASC,
         em.created_at ASC;
