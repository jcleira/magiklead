-- tenant_leads — per-tenant association with canonical persons.

-- AddTenantLead saves (or refreshes) a canonical person as this
-- tenant's lead. The `inserted` flag (Postgres xmax=0 trick) is TRUE
-- only when this row was a fresh INSERT — the handler reads it to
-- gate the leads_used quota increment so re-saving the same person
-- across campaigns doesn't double-count against the tenant's monthly
-- quota.
-- name: AddTenantLead :one
INSERT INTO tenant_leads (tenant_id, person_id, status, notes, added_by_user_id)
VALUES ($1, $2, COALESCE(sqlc.narg('status')::text, 'new'), $3, $4)
ON CONFLICT (tenant_id, person_id) DO UPDATE
    SET status           = COALESCE(EXCLUDED.status, tenant_leads.status),
        notes            = COALESCE(EXCLUDED.notes, tenant_leads.notes),
        added_by_user_id = COALESCE(EXCLUDED.added_by_user_id, tenant_leads.added_by_user_id)
RETURNING tenant_id, person_id, status, notes, added_at, added_by_user_id, (xmax = 0) AS inserted;

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
-- pass NULL to list all statuses. Title and company come from the
-- current job, and linkedin_url from the person's identifier — the URL
-- the LinkedIn rail invites. '' means none.
-- name: ListTenantLeads :many
WITH current_emp AS (
    SELECT DISTINCT ON (emp.person_id)
        emp.person_id,
        emp.title,
        emp.organization_id
    FROM employments emp
    WHERE emp.is_current = TRUE
    ORDER BY emp.person_id, emp.start_date DESC NULLS LAST
), li AS (
    SELECT DISTINCT ON (pi.person_id)
        pi.person_id,
        pi.identifier_value AS linkedin_url
    FROM person_identifiers pi
    WHERE pi.identifier_type = 'linkedin_url'
    ORDER BY pi.person_id
)
SELECT tl.tenant_id, tl.person_id, tl.status, tl.notes, tl.added_at, tl.added_by_user_id,
       p.canonical_name, p.first_name, p.last_name,
       COALESCE(ce.title, '')::text         AS title,
       COALESCE(o.canonical_name, '')::text AS company,
       COALESCE(li.linkedin_url, '')::text  AS linkedin_url
FROM tenant_leads tl
JOIN persons p ON p.id = tl.person_id
LEFT JOIN current_emp   ce ON ce.person_id = tl.person_id
LEFT JOIN organizations o  ON o.id = ce.organization_id
LEFT JOIN li               ON li.person_id = tl.person_id
WHERE tl.tenant_id = $1
  AND (sqlc.narg('status')::text IS NULL OR tl.status = sqlc.narg('status')::text)
ORDER BY tl.added_at DESC
LIMIT $2 OFFSET $3;

-- name: CountTenantLeads :one
SELECT COUNT(*) FROM tenant_leads
WHERE tenant_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status')::text);

-- ListTenantLeadsForExport dumps every saved-lead row for a tenant,
-- raw (no pagination, no JOIN). Used by the GDPR account-export
-- endpoint (issue #11) to write tenant_leads.json.
-- name: ListTenantLeadsForExport :many
SELECT * FROM tenant_leads
WHERE tenant_id = $1
ORDER BY added_at;
