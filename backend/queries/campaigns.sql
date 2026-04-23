-- name: CreateCampaign :one
INSERT INTO campaigns (tenant_id, play_id, name, status, gmail_account_id, sequence, linkedin_sequence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetCampaign :one
SELECT * FROM campaigns WHERE id = $1 AND tenant_id = $2;

-- name: GetCampaignByID :one
SELECT * FROM campaigns WHERE id = $1;

-- name: ListCampaigns :many
SELECT * FROM campaigns WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdateCampaignStatus :one
UPDATE campaigns SET status = $2 WHERE id = $1 RETURNING *;

-- name: UpdateCampaignStats :exec
UPDATE campaigns SET stats = $2 WHERE id = $1;

-- name: UpdateCampaignSequence :exec
UPDATE campaigns SET sequence = $2 WHERE id = $1;
