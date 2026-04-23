-- name: CreateTenant :one
INSERT INTO tenants (name, domain, business_profile) VALUES ($1, $2, $3) RETURNING *;

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: UpdateTenantProfile :one
UPDATE tenants SET business_profile = $2, updated_at = NOW() WHERE id = $1 RETURNING *;

-- name: ListTenantsByUser :many
SELECT t.* FROM tenants t
JOIN user_tenants ut ON t.id = ut.tenant_id
WHERE ut.user_id = $1;

-- name: CreateUserTenant :exec
INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, $3);
