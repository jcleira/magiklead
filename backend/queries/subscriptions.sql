-- name: CreateSubscription :one
INSERT INTO subscriptions (tenant_id, plan, leads_limit, sequences_limit, stripe_customer_id, stripe_subscription_id, current_period_start, current_period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE tenant_id = $1;

-- name: UpdateSubscriptionPlan :exec
UPDATE subscriptions SET plan = $2, leads_limit = $3, sequences_limit = $4, stripe_subscription_id = $5, current_period_start = $6, current_period_end = $7
WHERE tenant_id = $1;

-- name: UpdateStripeCustomer :exec
UPDATE subscriptions SET stripe_customer_id = $2 WHERE tenant_id = $1;

-- name: IncrementLeadsUsed :exec
UPDATE subscriptions SET leads_used = leads_used + $2 WHERE tenant_id = $1;

-- name: IncrementSequencesUsed :exec
UPDATE subscriptions SET sequences_used = sequences_used + $2 WHERE tenant_id = $1;

-- name: ResetUsageCounts :exec
UPDATE subscriptions SET leads_used = 0, sequences_used = 0
WHERE current_period_end < NOW();
