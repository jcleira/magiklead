-- name: CreateSubscription :one
INSERT INTO subscriptions (tenant_id, plan, leads_limit, sequences_limit, stripe_customer_id, stripe_subscription_id, current_period_start, current_period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE tenant_id = $1;

-- name: UpdateSubscriptionPlan :exec
UPDATE subscriptions SET plan = $2, leads_limit = $3, sequences_limit = $4, stripe_subscription_id = $5, current_period_start = $6, current_period_end = $7
WHERE tenant_id = $1;

-- UpdateSubscriptionByStripeID is called from customer.subscription.{created,updated}.
-- The webhook payload identifies the subscription by Stripe ID, not
-- tenant_id, so we match on stripe_subscription_id. Period dates flow
-- through unchanged from Stripe.
-- name: UpdateSubscriptionByStripeID :exec
UPDATE subscriptions
SET plan = $2, leads_limit = $3, sequences_limit = $4,
    current_period_start = $5,
    current_period_end = $6
WHERE stripe_subscription_id = $1;

-- DowngradeByStripeSubscriptionID finds the subscription linked to a
-- Stripe subscription ID and resets it to the given plan/limits.
-- Used by the customer.subscription.deleted webhook to flip a tenant
-- back to free without needing to know their tenant_id.
-- name: DowngradeByStripeSubscriptionID :exec
UPDATE subscriptions
SET plan = $2, leads_limit = $3, sequences_limit = $4,
    stripe_subscription_id = NULL,
    current_period_start = NULL,
    current_period_end = NULL
WHERE stripe_subscription_id = $1;

-- name: UpdateStripeCustomer :exec
UPDATE subscriptions SET stripe_customer_id = $2 WHERE tenant_id = $1;

-- LinkCheckoutSubscription runs from the checkout.session.completed
-- webhook handler. It writes the new plan + limits + stripe IDs in one
-- shot, identified by tenant_id (which we receive in session metadata).
-- Period dates are filled in by the subsequent
-- customer.subscription.{created,updated} event via
-- UpdateSubscriptionByStripeID; setting stripe_subscription_id here is
-- what lets that follow-up update find the row.
-- name: LinkCheckoutSubscription :exec
UPDATE subscriptions
SET plan = $2, leads_limit = $3, sequences_limit = $4,
    stripe_customer_id = $5,
    stripe_subscription_id = $6
WHERE tenant_id = $1;

-- name: IncrementLeadsUsed :exec
UPDATE subscriptions SET leads_used = leads_used + $2 WHERE tenant_id = $1;

-- name: IncrementSequencesUsed :exec
UPDATE subscriptions SET sequences_used = sequences_used + $2 WHERE tenant_id = $1;

-- name: ResetUsageCounts :exec
UPDATE subscriptions SET leads_used = 0, sequences_used = 0
WHERE current_period_end < NOW();
