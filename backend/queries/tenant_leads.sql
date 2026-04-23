-- tenant_leads — per-tenant association with canonical persons.

-- name: AddTenantLead :one
INSERT INTO tenant_leads (tenant_id, person_id, status, notes, added_by_user_id)
VALUES ($1, $2, COALESCE(sqlc.narg('status')::text, 'new'), $3, $4)
ON CONFLICT (tenant_id, person_id) DO UPDATE
    SET status           = COALESCE(EXCLUDED.status, tenant_leads.status),
        notes            = COALESCE(EXCLUDED.notes, tenant_leads.notes),
        added_by_user_id = COALESCE(EXCLUDED.added_by_user_id, tenant_leads.added_by_user_id)
RETURNING *;

-- name: GetTenantLead :one
SELECT * FROM tenant_leads
WHERE tenant_id = $1 AND person_id = $2;

-- name: RemoveTenantLead :exec
DELETE FROM tenant_leads WHERE tenant_id = $1 AND person_id = $2;

-- name: UpdateTenantLeadStatus :exec
UPDATE tenant_leads SET status = $3
WHERE tenant_id = $1 AND person_id = $2;

-- name: UpdateTenantLeadNotes :exec
UPDATE tenant_leads SET notes = $3
WHERE tenant_id = $1 AND person_id = $2;

-- ListTenantLeads returns this tenant's saved leads with the canonical
-- person attached, newest-first. Filter by status with `sqlc.narg`:
-- pass NULL to list all statuses.
-- name: ListTenantLeads :many
SELECT tl.tenant_id, tl.person_id, tl.status, tl.notes, tl.added_at, tl.added_by_user_id,
       p.canonical_name, p.first_name, p.last_name
FROM tenant_leads tl
JOIN persons p ON p.id = tl.person_id
WHERE tl.tenant_id = $1
  AND (sqlc.narg('status')::text IS NULL OR tl.status = sqlc.narg('status')::text)
ORDER BY tl.added_at DESC
LIMIT $2 OFFSET $3;

-- name: CountTenantLeads :one
SELECT COUNT(*) FROM tenant_leads
WHERE tenant_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);
