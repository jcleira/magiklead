-- name: CreateLead :one
INSERT INTO leads (linkedin_url, first_name, last_name, title, company, company_domain, location, email, email_verified, source, raw_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (linkedin_url) DO UPDATE SET
    title = EXCLUDED.title,
    company = EXCLUDED.company,
    email = COALESCE(EXCLUDED.email, leads.email),
    enriched_at = NOW()
RETURNING *;

-- name: GetLead :one
SELECT * FROM leads WHERE id = $1;

-- name: ListLeadsByIDs :many
SELECT * FROM leads WHERE id = ANY($1::uuid[]);

-- name: ListLeads :many
SELECT * FROM leads
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: SearchLeads :many
SELECT * FROM leads
WHERE (first_name ILIKE '%' || $1 || '%' OR last_name ILIKE '%' || $1 || '%' OR company ILIKE '%' || $1 || '%' OR email ILIKE '%' || $1 || '%')
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
