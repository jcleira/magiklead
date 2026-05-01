-- Pending erasure requests awaiting confirmation. Plan §T15.

-- name: InsertPrivacyRequest :one
INSERT INTO privacy_requests (
    token_hash, email, linkedin_url, name, company, reason, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetPrivacyRequestByTokenHash :one
SELECT * FROM privacy_requests WHERE token_hash = $1;

-- name: ConfirmPrivacyRequest :exec
UPDATE privacy_requests
SET confirmed_at = NOW(), deleted_persons = $2
WHERE id = $1;
