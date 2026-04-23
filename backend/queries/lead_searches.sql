-- name: GetCachedSearch :one
SELECT * FROM lead_searches
WHERE query_hash = $1
AND fetched_at > NOW() - (ttl_days || ' days')::interval
ORDER BY fetched_at DESC
LIMIT 1;

-- name: CreateLeadSearch :one
INSERT INTO lead_searches (query_hash, filters, result_lead_ids, fetched_at)
VALUES ($1, $2, $3, NOW())
RETURNING *;
