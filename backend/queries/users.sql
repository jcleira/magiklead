-- name: CreateUser :one
INSERT INTO users (clerk_id, email, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetUserByClerkID :one
SELECT * FROM users WHERE clerk_id = $1;

-- name: UpdateUser :one
UPDATE users SET email = $2, name = $3, updated_at = NOW() WHERE clerk_id = $1 RETURNING *;

-- UpsertUserByClerkID is the bootstrap-path write: it inserts a new
-- user when the clerk_id is unseen and refreshes email/name on every
-- subsequent call, returning the row either way. The conflict target
-- exploits the UNIQUE constraint on clerk_id, which makes the path
-- safe under concurrent first-requests (one wins the insert, the
-- others see an UPDATE with their refreshed claims).
-- name: UpsertUserByClerkID :one
INSERT INTO users (clerk_id, email, name)
VALUES ($1, $2, $3)
ON CONFLICT (clerk_id) DO UPDATE
SET email = EXCLUDED.email,
    name = EXCLUDED.name,
    updated_at = NOW()
RETURNING *;

-- LockUserByID takes a row-level lock on a user row inside a tx so the
-- rest of the bootstrap (tenant, link, subscription) is serialised for
-- that user. Pair it with UpsertUserByClerkID in a single transaction.
-- name: LockUserByID :one
SELECT * FROM users WHERE id = $1 FOR UPDATE;
