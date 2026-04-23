-- Admin panel queries — conflict review, person inspection, merge,
-- and hard delete. Plan §T14.
--
-- The merge endpoint runs a multi-statement transaction in the
-- handler (see internal/handler/admin.go). Each reassignment query
-- below is shaped so the handler can execute them in order under one
-- pgx.Tx and have the FK-bearing rows re-pointed without violating
-- the unique constraints in 015_alias_uniqueness.

-- ============================================================================
-- Conflict queue
-- ============================================================================

-- ListConflicts returns the conflict queue, newest first, with the
-- linked canonical person's name and the source the new record came
-- from. Filter by status with sqlc.narg('status') = NULL for "all".
-- name: ListConflicts :many
SELECT
    cq.id,
    cq.canonical_table,
    cq.canonical_id,
    cq.new_source_record_id,
    cq.status,
    cq.created_at,
    p.canonical_name AS person_canonical_name,
    sr.fields        AS source_record_fields,
    s.name           AS source_name
FROM conflict_queue cq
JOIN source_records sr ON sr.id = cq.new_source_record_id
JOIN sources s         ON s.id = sr.source_id
LEFT JOIN persons p ON cq.canonical_table = 'persons' AND p.id = cq.canonical_id
WHERE (sqlc.narg('status')::text IS NULL OR cq.status = sqlc.narg('status')::text)
ORDER BY cq.created_at DESC
LIMIT $1 OFFSET $2;

-- name: GetConflict :one
SELECT id, canonical_table, canonical_id, new_source_record_id, status, created_at
FROM conflict_queue
WHERE id = $1;

-- name: UpdateConflictStatus :exec
UPDATE conflict_queue SET status = $2 WHERE id = $1;

-- ============================================================================
-- Person detail (a handful of focused queries — handler aggregates)
-- ============================================================================

-- name: GetPerson :one
SELECT id, canonical_name, first_name, last_name, normalized_name, created_at, updated_at
FROM persons
WHERE id = $1;

-- name: ListPersonAliases :many
SELECT id, alias, alias_type FROM person_aliases
WHERE person_id = $1
ORDER BY alias;

-- name: ListPersonIdentifiers :many
SELECT id, identifier_type, identifier_value, is_primary FROM person_identifiers
WHERE person_id = $1
ORDER BY identifier_type, identifier_value;

-- name: ListPersonEmails :many
SELECT id, email, verified_at, verification_method, bounce_count, is_catchall, created_at
FROM emails
WHERE person_id = $1
ORDER BY (verified_at IS NOT NULL) DESC, bounce_count ASC, created_at ASC;

-- name: ListPersonPhones :many
SELECT id, phone, verified_at, created_at FROM phones
WHERE person_id = $1
ORDER BY created_at;

-- name: ListPersonSocialProfiles :many
SELECT id, platform, handle, url, created_at FROM social_profiles
WHERE person_id = $1
ORDER BY platform;

-- name: ListPersonEmployments :many
SELECT
    e.id, e.title, e.start_date, e.end_date, e.is_current, e.created_at,
    o.id            AS organization_id,
    o.canonical_name AS organization_name,
    o.primary_domain
FROM employments e
JOIN organizations o ON o.id = e.organization_id
WHERE e.person_id = $1
ORDER BY e.is_current DESC, e.start_date DESC NULLS LAST;

-- ListPersonEvidence joins each evidence row to its source_record
-- and the originating source so the admin can see "where did this
-- claim come from?" without hitting raw_ingests directly.
-- name: ListPersonEvidence :many
SELECT
    ev.id,
    ev.confidence,
    ev.conflict_flag,
    ev.created_at,
    ev.source_record_id,
    sr.external_id,
    sr.fields,
    sr.ingested_at,
    s.id   AS source_id,
    s.name AS source_name,
    s.type AS source_type
FROM evidence ev
JOIN source_records sr ON sr.id = ev.source_record_id
JOIN sources s         ON s.id = sr.source_id
WHERE ev.canonical_table = 'persons' AND ev.canonical_id = $1
ORDER BY ev.created_at DESC;

-- ============================================================================
-- Merge persons (run inside one pgx.Tx in the handler)
-- ============================================================================
--
-- Step order matters: aliases / identifiers / social_profiles /
-- tenant_leads have unique constraints that the merged person may
-- already satisfy, so we copy what doesn't conflict and then drop
-- the originals before flipping the simpler FK tables.

-- name: MoveAliasesForMerge :exec
INSERT INTO person_aliases (person_id, alias, alias_type)
SELECT sqlc.arg('keep_id')::uuid, alias, alias_type
FROM person_aliases
WHERE person_id = sqlc.arg('merged_id')::uuid
ON CONFLICT ON CONSTRAINT person_aliases_unique DO NOTHING;

-- name: DeleteAliasesForMerged :exec
DELETE FROM person_aliases WHERE person_id = $1;

-- person_identifiers.identifier_value is globally UNIQUE, so the
-- insert-then-delete trick used for aliases would have ON CONFLICT
-- skip the new row and then DELETE drop the original — losing the
-- identifier. Repoint via UPDATE instead, only for values the
-- survivor doesn't already have. Anything the survivor already
-- carries is left to the DELETE.
-- name: MoveIdentifiersForMerge :exec
UPDATE person_identifiers
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid
  AND NOT EXISTS (
    SELECT 1 FROM person_identifiers ki
    WHERE ki.person_id = sqlc.arg('keep_id')::uuid
      AND ki.identifier_value = person_identifiers.identifier_value
  );

-- name: DeleteIdentifiersForMerged :exec
DELETE FROM person_identifiers WHERE person_id = $1;

-- social_profiles.(platform, url) is globally UNIQUE — same shape as
-- identifiers, so use the same UPDATE-WHERE-NOT-EXISTS pattern.
-- name: MoveSocialProfilesForMerge :exec
UPDATE social_profiles
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid
  AND NOT EXISTS (
    SELECT 1 FROM social_profiles ks
    WHERE ks.person_id = sqlc.arg('keep_id')::uuid
      AND ks.platform = social_profiles.platform
      AND ks.url = social_profiles.url
  );

-- name: DeleteSocialProfilesForMerged :exec
DELETE FROM social_profiles WHERE person_id = $1;

-- tenant_leads: PRIMARY KEY (tenant_id, person_id) → re-point with
-- ON CONFLICT to keep both saves merged into the surviving person.
-- name: MoveTenantLeadsForMerge :exec
INSERT INTO tenant_leads (tenant_id, person_id, status, notes, added_at, added_by_user_id)
SELECT tenant_id, sqlc.arg('keep_id')::uuid, status, notes, added_at, added_by_user_id
FROM tenant_leads
WHERE person_id = sqlc.arg('merged_id')::uuid
ON CONFLICT (tenant_id, person_id) DO NOTHING;

-- name: DeleteTenantLeadsForMerged :exec
DELETE FROM tenant_leads WHERE person_id = $1;

-- evidence / employments / emails / phones have no in-table unique
-- collisions on person_id, so a plain repoint suffices.

-- name: ReassignEvidenceForMerge :exec
UPDATE evidence
SET canonical_id = sqlc.arg('keep_id')::uuid
WHERE canonical_table = 'persons'
  AND canonical_id = sqlc.arg('merged_id')::uuid;

-- name: ReassignEmploymentsForMerge :exec
UPDATE employments
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid;

-- name: ReassignEmailsForMerge :exec
UPDATE emails
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid;

-- name: ReassignPhonesForMerge :exec
UPDATE phones
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid;

-- The legacy `leads` and `campaign_leads` tables both carry a
-- nullable person_id bridge column (migrations 013 and 016). Repoint
-- them too so existing per-tenant lead histories survive a merge.

-- name: ReassignCampaignLeadsForMerge :exec
UPDATE campaign_leads
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid;

-- name: ReassignLegacyLeadsForMerge :exec
UPDATE leads
SET person_id = sqlc.arg('keep_id')::uuid
WHERE person_id = sqlc.arg('merged_id')::uuid;

-- name: CreateMergeHistory :one
INSERT INTO merge_history (surviving_id, merged_id, canonical_table, merged_by_user_id, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- DeletePerson hard-deletes the canonical person row. CASCADE on
-- person_id FKs (employments, emails, etc.) takes the rest. Used by
-- both the merge flow (final step, removes the merged_id) and the
-- GDPR-style admin delete endpoint.
-- name: DeletePerson :exec
DELETE FROM persons WHERE id = $1;
