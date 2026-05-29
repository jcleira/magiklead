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

-- The DeleteAccount* family below is the explicit FK-respecting
-- cascade used by the authenticated account-delete endpoint (issue
-- #11). The schema mixes ON DELETE CASCADE (tenant_leads, unsubscribes)
-- with plain FKs (campaigns, plays, gmail_accounts, subscriptions,
-- email_accounts, user_tenants). Rather than introduce a migration
-- altering every FK, the handler runs these in dependency order inside
-- one transaction — same pattern the privacy/erasure path already uses
-- for canonical-graph cleanup.
--
-- Order assumed by callers:
--   1. DeleteEmailEventsByTenant     -- email_events → campaign_leads → campaigns → tenant
--   2. DeleteCampaignLeadsByTenant   -- campaign_leads → campaigns → tenant
--   3. DeleteCampaignsByTenant       -- campaigns → tenant
--   4. DeletePlaysByTenant
--   5. DeleteGmailAccountsByTenant
--   6. DeleteEmailAccountsByTenant
--   7. DeleteSubscriptionByTenant
--   8. DeleteUserTenantsByTenant
--   9. DeleteTenantByID              -- cascades tenant_leads + unsubscribes
--  10. DeleteUserByIDIfOrphan        -- only deletes the user if no other tenant memberships remain

-- name: DeleteEmailEventsByTenant :exec
DELETE FROM email_events
WHERE campaign_lead_id IN (
    SELECT cl.id FROM campaign_leads cl
    JOIN campaigns c ON c.id = cl.campaign_id
    WHERE c.tenant_id = $1
);

-- name: DeleteCampaignLeadsByTenant :exec
DELETE FROM campaign_leads
WHERE campaign_id IN (SELECT id FROM campaigns WHERE tenant_id = $1);

-- name: DeleteCampaignsByTenant :exec
DELETE FROM campaigns WHERE tenant_id = $1;

-- name: DeletePlaysByTenant :exec
DELETE FROM plays WHERE tenant_id = $1;

-- name: DeleteGmailAccountsByTenant :exec
DELETE FROM gmail_accounts WHERE tenant_id = $1;

-- name: DeleteEmailAccountsByTenant :exec
DELETE FROM email_accounts WHERE tenant_id = $1;

-- name: DeleteSubscriptionByTenant :exec
DELETE FROM subscriptions WHERE tenant_id = $1;

-- name: DeleteUserTenantsByTenant :exec
DELETE FROM user_tenants WHERE tenant_id = $1;

-- name: DeleteTenantByID :exec
DELETE FROM tenants WHERE id = $1;

-- DeleteUserByIDIfOrphan removes a user row only if no user_tenants
-- entries remain. Defensive against the (out-of-scope) case where a
-- user belongs to more than one tenant — we never orphan the user
-- from a still-referenced tenant. Today every user owns exactly one
-- tenant, so the row deletes whenever the caller has just dropped
-- that user's only user_tenants link.
-- name: DeleteUserByIDIfOrphan :exec
DELETE FROM users
WHERE id = $1
  AND NOT EXISTS (SELECT 1 FROM user_tenants WHERE user_id = $1);
