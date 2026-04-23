-- name: ListUnverifiedEmails :many
SELECT * FROM emails
WHERE verified_at IS NULL
ORDER BY created_at ASC
LIMIT $1;

-- name: UpdateEmailVerification :exec
UPDATE emails SET
    verified_at = NOW(),
    verification_method = $2,
    bounce_count = $3,
    is_catchall = $4
WHERE id = $1;

-- name: GetDomainByName :one
SELECT * FROM domains WHERE domain = $1;

-- name: UpsertDomain :one
INSERT INTO domains (domain, mx_records, is_catchall, last_verified_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (domain) DO UPDATE
    SET mx_records      = EXCLUDED.mx_records,
        -- Preserve a previously-established catch-all flag if the new
        -- probe didn't run (NULL means "unknown").
        is_catchall      = COALESCE(EXCLUDED.is_catchall, domains.is_catchall),
        last_verified_at = NOW()
RETURNING *;

-- name: UpdateDomainCatchall :exec
UPDATE domains SET
    is_catchall = $2,
    last_verified_at = NOW()
WHERE domain = $1;

-- name: CreateEmail :one
INSERT INTO emails (email, person_id, verification_method)
VALUES ($1, $2, $3)
ON CONFLICT (email) DO UPDATE
    SET person_id = COALESCE(emails.person_id, EXCLUDED.person_id)
RETURNING *;
