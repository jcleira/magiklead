package handler

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"

	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// PlanLimits maps plan names to lead and sequence limits.
var PlanLimits = map[string]struct {
	Leads     int32
	Sequences int32
}{
	"free":    {100, 300},
	"starter": {500, 1500},
	"growth":  {2000, 6000},
	"scale":   {10000, 30000},
}

// pricedPlans is the set of plans that have a Stripe price (i.e.
// non-free). Drives env-var resolution at startup; iteration order
// doesn't matter.
var pricedPlans = []string{"starter", "growth", "scale"}

// billingQueries is the slice of repository.Queries the billing
// handler actually uses. The interface seam keeps the handler
// testable without a DB — production wires the real *repository.Queries.
type billingQueries interface {
	GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error)
	UpdateStripeCustomer(ctx context.Context, arg repository.UpdateStripeCustomerParams) error
	UpdateSubscriptionByStripeID(ctx context.Context, arg repository.UpdateSubscriptionByStripeIDParams) error
	DowngradeByStripeSubscriptionID(ctx context.Context, arg repository.DowngradeByStripeSubscriptionIDParams) error
	LinkCheckoutSubscription(ctx context.Context, arg repository.LinkCheckoutSubscriptionParams) error
}

type BillingHandler struct {
	queries       billingQueries
	frontendURL   string
	webhookSecret string
	// priceToPlan maps a Stripe price ID to a local plan name. Built
	// from STRIPE_PRICE_<PLAN> env vars at construction; missing
	// entries cause subscription.{created,updated} to log and skip
	// rather than guess a plan.
	priceToPlan map[string]string
	// planToPrice is the inverse of priceToPlan — drives CreateCheckout's
	// plan-name → price ID resolution so the frontend never has to embed
	// Stripe identifiers.
	planToPrice map[string]string
}

func NewBillingHandler(q *repository.Queries, stripeKey, webhookSecret, frontendURL string) *BillingHandler {
	stripe.Key = stripeKey
	priceToPlan := make(map[string]string, len(pricedPlans))
	planToPrice := make(map[string]string, len(pricedPlans))
	for _, plan := range pricedPlans {
		envKey := "STRIPE_PRICE_" + strings.ToUpper(plan)
		if priceID := strings.TrimSpace(os.Getenv(envKey)); priceID != "" {
			priceToPlan[priceID] = plan
			planToPrice[plan] = priceID
		}
	}
	return &BillingHandler{
		queries:       q,
		frontendURL:   frontendURL,
		webhookSecret: webhookSecret,
		priceToPlan:   priceToPlan,
		planToPrice:   planToPrice,
	}
}

// CreateCheckout handles POST /api/v1/billing/checkout.
//
// Accepts either:
//
//   - `plan` (preferred): one of starter/growth/scale; backend resolves
//     to the configured Stripe Price ID via STRIPE_PRICE_<PLAN>. The
//     frontend never embeds Stripe identifiers.
//   - `price_id` (legacy): the literal Stripe Price ID; used by code
//     paths that already know which price to bill.
//
// Sets AllowPromotionCodes so testers can apply the 100%-off comp code
// directly on the Stripe Checkout page (no server-side promo state).
// Embeds tenant_id + plan in session metadata — the webhook reads both
// to update the subscription row synchronously when the session
// completes.
func (h *BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PriceID string `json:"price_id"`
		Plan    string `json:"plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid request body"})
		return
	}

	plan := strings.ToLower(strings.TrimSpace(req.Plan))
	priceID := strings.TrimSpace(req.PriceID)

	// Plan name → price ID resolution. The frontend's preferred path is
	// to send `plan`; the legacy `price_id` form is still accepted for
	// any caller that knows the Stripe identifier directly.
	if plan != "" {
		resolved, ok := h.planToPrice[plan]
		if !ok {
			apierr.WriteError(w, apierr.APIError{Status: 400, Code: "unknown_plan", Message: "plan not configured: " + plan})
			return
		}
		priceID = resolved
	} else if priceID != "" {
		// If we recognise the price ID, fill plan back in so the webhook
		// metadata round-trip stays useful.
		if p, ok := h.priceToPlan[priceID]; ok {
			plan = p
		}
	}

	if priceID == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "plan or price_id is required"})
		return
	}

	tenantID := getTenantID(r.Context())

	// Get or create Stripe customer
	sub, err := h.queries.GetSubscription(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
		},
		AllowPromotionCodes: stripe.Bool(true),
		SuccessURL:          stripe.String(h.frontendURL + "/settings?checkout=success"),
		CancelURL:           stripe.String(h.frontendURL + "/settings?checkout=cancel"),
		Metadata: map[string]string{
			"tenant_id": tenantID.String(),
			"plan":      plan,
		},
	}

	if sub.StripeCustomerID.Valid && sub.StripeCustomerID.String != "" {
		params.Customer = stripe.String(sub.StripeCustomerID.String)
	}

	s, err := checkoutsession.New(params)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "stripe_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"url": s.URL})
}

// CreatePortal handles POST /api/v1/billing/portal
func (h *BillingHandler) CreatePortal(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	sub, err := h.queries.GetSubscription(r.Context(), pgUUID(tenantID))
	if err != nil || !sub.StripeCustomerID.Valid {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "no_subscription", Message: "No active subscription"})
		return
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(sub.StripeCustomerID.String),
		ReturnURL: stripe.String(h.frontendURL + "/settings"),
	}
	s, err := session.New(params)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "stripe_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"url": s.URL})
}

// GetSubscription handles GET /api/v1/billing/subscription
func (h *BillingHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	sub, err := h.queries.GetSubscription(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, sub)
}

// HandleWebhook handles POST /api/v1/webhooks/stripe
func (h *BillingHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.webhookSecret == "" {
		// Refuse to process unsigned traffic — prod must set
		// STRIPE_WEBHOOK_SECRET. This mirrors the Clerk webhook.
		log.Print("stripe webhook rejected: STRIPE_WEBHOOK_SECRET not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	event, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), h.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		log.Printf("Stripe webhook signature verification failed: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var cs stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &cs); err != nil {
			log.Printf("stripe: failed to unmarshal checkout session: %v", err)
			break
		}
		h.handleCheckoutCompleted(r.Context(), &cs)

	case "customer.subscription.created", "customer.subscription.updated":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			log.Printf("stripe: failed to unmarshal subscription: %v", err)
			break
		}
		h.handleSubscriptionUpsert(r.Context(), &sub)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			break
		}
		h.handleSubscriptionDeleted(r.Context(), &sub)

	default:
		// Stripe sends every enabled event type to every webhook.
		// Refunds, disputes, invoice.* — we don't act on them today
		// but log so they're searchable when something looks wrong.
		log.Printf("stripe webhook: ignoring event type %q id=%s", event.Type, event.ID)
	}

	w.WriteHeader(http.StatusOK)
}

// handleCheckoutCompleted updates the tenant's subscription row to
// reflect the plan they just paid for — in one query, synchronously,
// without waiting for the follow-up customer.subscription.created event
// (the older two-event flow had a race: handleSubscriptionUpsert keys
// on stripe_subscription_id, but that column was still NULL until
// customer.subscription.created landed, so the plan never updated).
// Plan name flows in via session metadata set by CreateCheckout. The
// follow-up subscription.{created,updated} event still runs and is
// what fills in the period start/end dates.
func (h *BillingHandler) handleCheckoutCompleted(ctx context.Context, cs *stripe.CheckoutSession) {
	tenantIDStr, ok := cs.Metadata["tenant_id"]
	if !ok {
		log.Print("stripe: checkout.session.completed without tenant_id metadata; ignoring")
		return
	}
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		log.Printf("stripe: invalid tenant_id metadata %q: %v", tenantIDStr, err)
		return
	}

	plan := cs.Metadata["plan"]
	limits, knownPlan := PlanLimits[plan]
	if !knownPlan {
		// Fall back to recording just the Stripe customer ID so the
		// plan update can land via customer.subscription.created. This
		// is the legacy path for any session created before plan
		// metadata was introduced.
		if cs.Customer != nil {
			if err := h.queries.UpdateStripeCustomer(ctx, repository.UpdateStripeCustomerParams{
				TenantID:         pgUUID(tenantID),
				StripeCustomerID: pgtype.Text{String: cs.Customer.ID, Valid: true},
			}); err != nil {
				log.Printf("stripe: update customer for tenant %s: %v", tenantID, err)
			}
		}
		log.Printf("stripe: checkout.session.completed for tenant %s missing/unknown plan metadata %q; recorded customer only", tenantID, plan)
		return
	}

	params := repository.LinkCheckoutSubscriptionParams{
		TenantID:       pgUUID(tenantID),
		Plan:           plan,
		LeadsLimit:     limits.Leads,
		SequencesLimit: limits.Sequences,
	}
	if cs.Customer != nil && cs.Customer.ID != "" {
		params.StripeCustomerID = pgtype.Text{String: cs.Customer.ID, Valid: true}
	}
	if cs.Subscription != nil && cs.Subscription.ID != "" {
		params.StripeSubscriptionID = pgtype.Text{String: cs.Subscription.ID, Valid: true}
	}

	if err := h.queries.LinkCheckoutSubscription(ctx, params); err != nil {
		log.Printf("stripe: link checkout subscription for tenant %s plan %s: %v", tenantID, plan, err)
	}
}

// handleSubscriptionUpsert is called from customer.subscription.created
// (initial assignment after checkout) and customer.subscription.updated
// (plan changes via the Stripe customer portal). The price ID on the
// first item drives the plan lookup; missing prices log and no-op so a
// dashboard misconfiguration doesn't silently downgrade the customer.
func (h *BillingHandler) handleSubscriptionUpsert(ctx context.Context, sub *stripe.Subscription) {
	if sub.Items == nil || len(sub.Items.Data) == 0 || sub.Items.Data[0].Price == nil {
		log.Printf("stripe: subscription %s has no priced items; skipping", sub.ID)
		return
	}
	priceID := sub.Items.Data[0].Price.ID
	plan, ok := h.priceToPlan[priceID]
	if !ok {
		log.Printf("stripe: subscription %s has unknown price_id %q; set STRIPE_PRICE_<PLAN> env to map it", sub.ID, priceID)
		return
	}
	limits := PlanLimits[plan]

	if err := h.queries.UpdateSubscriptionByStripeID(ctx, repository.UpdateSubscriptionByStripeIDParams{
		StripeSubscriptionID: pgtype.Text{String: sub.ID, Valid: true},
		Plan:                 plan,
		LeadsLimit:           limits.Leads,
		SequencesLimit:       limits.Sequences,
		CurrentPeriodStart:   timestamptzFromUnix(sub.CurrentPeriodStart),
		CurrentPeriodEnd:     timestamptzFromUnix(sub.CurrentPeriodEnd),
	}); err != nil {
		log.Printf("stripe: update subscription %s → plan %s: %v", sub.ID, plan, err)
	}
}

func (h *BillingHandler) handleSubscriptionDeleted(ctx context.Context, sub *stripe.Subscription) {
	freeLimits := PlanLimits["free"]
	if err := h.queries.DowngradeByStripeSubscriptionID(ctx, repository.DowngradeByStripeSubscriptionIDParams{
		StripeSubscriptionID: pgtype.Text{String: sub.ID, Valid: true},
		Plan:                 "free",
		LeadsLimit:           freeLimits.Leads,
		SequencesLimit:       freeLimits.Sequences,
	}); err != nil {
		log.Printf("stripe: downgrade-by-sub-id failed for %s: %v", sub.ID, err)
	}
}

func timestamptzFromUnix(sec int64) pgtype.Timestamptz {
	if sec == 0 {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: time.Unix(sec, 0).UTC(), Valid: true}
}
