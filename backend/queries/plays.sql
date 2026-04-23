-- name: CreatePlay :one
INSERT INTO plays (tenant_id, name, description, icp, search_query, channels, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetPlay :one
SELECT * FROM plays WHERE id = $1 AND tenant_id = $2;

-- name: ListPlays :many
SELECT * FROM plays WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdatePlay :one
UPDATE plays SET name = $2, description = $3, icp = $4, search_query = $5, channels = $6, status = $7
WHERE id = $1 RETURNING *;

-- name: DeletePlay :exec
DELETE FROM plays WHERE id = $1 AND tenant_id = $2;
