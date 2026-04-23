-- name: CreateUser :one
INSERT INTO users (clerk_id, email, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetUserByClerkID :one
SELECT * FROM users WHERE clerk_id = $1;

-- name: UpdateUser :one
UPDATE users SET email = $2, name = $3, updated_at = NOW() WHERE clerk_id = $1 RETURNING *;
