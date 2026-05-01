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

type BillingHandler struct {
	queries       *repository.Queries
	frontendURL   string
	webhookSecret string
	// priceToPlan maps a Stripe price ID to a local plan name. Built
	// from STRIPE_PRICE_<PLAN> env vars at construction; missing
	// entries cause subscription.{created,updated} to log and skip
	// rather than guess a plan.
	priceToPlan map[string]string
}

func NewBillingHandler(q *repository.Queries, stripeKey, webhookSecret, frontendURL string) *BillingHandler {
	stripe.Key = stripeKey
	priceToPlan := make(map[string]string, len(pricedPlans))
	for _, plan := range pricedPlans {
		envKey := "STRIPE_PRICE_" + strings.ToUpper(plan)
		if priceID := strings.TrimSpace(os.Getenv(envKey)); priceID != "" {
			priceToPlan[priceID] = plan
		}
	}
	return &BillingHandler{
		queries:       q,
		frontendURL:   frontendURL,
		webhookSecret: webhookSecret,
		priceToPlan:   priceToPlan,
	}
}

// CreateCheckout handles POST /api/v1/billing/checkout
func (h *BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PriceID string `json:"price_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PriceID == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "price_id is required"})
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
			{Price: stripe.String(req.PriceID), Quantity: stripe.Int64(1)},
		},
		SuccessURL: stripe.String(h.frontendURL + "/settings?checkout=success"),
		CancelURL:  stripe.String(h.frontendURL + "/settings?checkout=cancel"),
		Metadata: map[string]string{
			"tenant_id": tenantID.String(),
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
	}

	w.WriteHeader(http.StatusOK)
}

// handleCheckoutCompleted records the Stripe customer + subscription
// IDs against the tenant. Plan determination is deferred to
// customer.subscription.{created,updated} which carries the items the
// session doesn't expand by default.
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

	if cs.Customer != nil {
		if err := h.queries.UpdateStripeCustomer(ctx, repository.UpdateStripeCustomerParams{
			TenantID:         pgUUID(tenantID),
			StripeCustomerID: pgtype.Text{String: cs.Customer.ID, Valid: true},
		}); err != nil {
			log.Printf("stripe: update customer for tenant %s: %v", tenantID, err)
		}
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
