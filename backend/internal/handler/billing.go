package handler

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

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

type BillingHandler struct {
	queries     *repository.Queries
	frontendURL string
}

func NewBillingHandler(q *repository.Queries, stripeKey, frontendURL string) *BillingHandler {
	stripe.Key = stripeKey
	return &BillingHandler{queries: q, frontendURL: frontendURL}
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

	event, err := webhook.ConstructEventWithOptions(body, r.Header.Get("Stripe-Signature"), "", webhook.ConstructEventOptions{
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
			log.Printf("Failed to unmarshal checkout session: %v", err)
			break
		}
		h.handleCheckoutCompleted(r.Context(), &cs)

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			break
		}
		h.handleSubscriptionDeleted(r.Context(), &sub)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BillingHandler) handleCheckoutCompleted(ctx context.Context, cs *stripe.CheckoutSession) {
	tenantIDStr, ok := cs.Metadata["tenant_id"]
	if !ok {
		return
	}

	// Determine plan from the subscription amount
	plan := "starter"
	limits := PlanLimits[plan]

	if cs.Subscription != nil {
		h.queries.UpdateSubscriptionPlan(ctx, repository.UpdateSubscriptionPlanParams{
			TenantID:             pgtype.UUID{Bytes: parseUUID(tenantIDStr), Valid: true},
			Plan:                 plan,
			LeadsLimit:           limits.Leads,
			SequencesLimit:       limits.Sequences,
			StripeSubscriptionID: pgtype.Text{String: cs.Subscription.ID, Valid: true},
		})
	}

	if cs.Customer != nil {
		h.queries.UpdateStripeCustomer(ctx, repository.UpdateStripeCustomerParams{
			TenantID:         pgtype.UUID{Bytes: parseUUID(tenantIDStr), Valid: true},
			StripeCustomerID: pgtype.Text{String: cs.Customer.ID, Valid: true},
		})
	}
}

func (h *BillingHandler) handleSubscriptionDeleted(ctx context.Context, sub *stripe.Subscription) {
	// Downgrade to free — find tenant by stripe subscription ID
	freeLimits := PlanLimits["free"]
	h.queries.UpdateSubscriptionPlan(ctx, repository.UpdateSubscriptionPlanParams{
		Plan:                 "free",
		LeadsLimit:           freeLimits.Leads,
		SequencesLimit:       freeLimits.Sequences,
		StripeSubscriptionID: pgtype.Text{String: sub.ID, Valid: true},
	})
}

func parseUUID(s string) [16]byte {
	var b [16]byte
	// Simple hex parse for UUID
	s = s[0:8] + s[9:13] + s[14:18] + s[19:23] + s[24:36]
	for i := 0; i < 16; i++ {
		b[i] = hexToByte(s[i*2], s[i*2+1])
	}
	return b
}

func hexToByte(h, l byte) byte {
	return (hexVal(h) << 4) | hexVal(l)
}

func hexVal(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10
	default:
		return 0
	}
}
