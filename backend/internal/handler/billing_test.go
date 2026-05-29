package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// captureLog redirects the standard logger to `dst` and returns a
// reset func that the caller defers. Used to assert handlers logged
// something — e.g. that an irrelevant Stripe event left a trail.
func captureLog(t *testing.T, dst io.Writer) func() {
	t.Helper()
	prev := log.Writer()
	log.SetOutput(dst)
	return func() { log.SetOutput(prev) }
}

// fakeBillingQueries records every call against the billingQueries
// interface. Each method is one line of "what was asked" + a canned
// response — same shape as fakeSettingsQueries.
type fakeBillingQueries struct {
	sub    repository.Subscription
	subErr error

	linkCalls         []repository.LinkCheckoutSubscriptionParams
	linkErr           error
	updateCustCalls   []repository.UpdateStripeCustomerParams
	updateByStripe    []repository.UpdateSubscriptionByStripeIDParams
	downgradeCalls    []repository.DowngradeByStripeSubscriptionIDParams
}

func (f *fakeBillingQueries) GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error) {
	return f.sub, f.subErr
}

func (f *fakeBillingQueries) UpdateStripeCustomer(ctx context.Context, arg repository.UpdateStripeCustomerParams) error {
	f.updateCustCalls = append(f.updateCustCalls, arg)
	return nil
}

func (f *fakeBillingQueries) UpdateSubscriptionByStripeID(ctx context.Context, arg repository.UpdateSubscriptionByStripeIDParams) error {
	f.updateByStripe = append(f.updateByStripe, arg)
	return nil
}

func (f *fakeBillingQueries) DowngradeByStripeSubscriptionID(ctx context.Context, arg repository.DowngradeByStripeSubscriptionIDParams) error {
	f.downgradeCalls = append(f.downgradeCalls, arg)
	return nil
}

func (f *fakeBillingQueries) LinkCheckoutSubscription(ctx context.Context, arg repository.LinkCheckoutSubscriptionParams) error {
	f.linkCalls = append(f.linkCalls, arg)
	return f.linkErr
}

// signStripePayload emits a Stripe-Signature header for `payload`
// using `secret`. Mirrors the v1 scheme Stripe uses in production
// (timestamp + "." + payload, HMAC-SHA256 in hex).
func signStripePayload(t *testing.T, payload, secret string) string {
	t.Helper()
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s", ts, payload)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestNewBillingHandler_LoadsPriceMap(t *testing.T) {
	t.Setenv("STRIPE_PRICE_STARTER", "price_starter_test")
	t.Setenv("STRIPE_PRICE_GROWTH", "price_growth_test")
	t.Setenv("STRIPE_PRICE_SCALE", "price_scale_test")

	h := NewBillingHandler(nil, "sk_test_xxx", "whsec_xxx", "http://localhost")

	cases := map[string]string{
		"price_starter_test": "starter",
		"price_growth_test":  "growth",
		"price_scale_test":   "scale",
	}
	for priceID, wantPlan := range cases {
		got, ok := h.priceToPlan[priceID]
		if !ok {
			t.Errorf("price %q missing from map", priceID)
			continue
		}
		if got != wantPlan {
			t.Errorf("price %q → %q, want %q", priceID, got, wantPlan)
		}
	}

	// Unset env vars should yield an empty map (no false-positive
	// "starter" mapping like the old hardcoded behaviour).
	if _, ok := h.priceToPlan["price_unknown"]; ok {
		t.Error("unconfigured price ID resolved to a plan — must not happen")
	}
}

func TestNewBillingHandler_EmptyEnvSkipsPlan(t *testing.T) {
	t.Setenv("STRIPE_PRICE_STARTER", "")
	t.Setenv("STRIPE_PRICE_GROWTH", "")
	t.Setenv("STRIPE_PRICE_SCALE", "")

	h := NewBillingHandler(nil, "", "", "")
	if len(h.priceToPlan) != 0 {
		t.Errorf("priceToPlan=%v, want empty when no env vars set", h.priceToPlan)
	}
}

func TestStripeWebhook_RejectsUnconfiguredSecret(t *testing.T) {
	// An empty webhook secret must fail closed — production must not
	// silently accept signed traffic when verification is disabled.
	h := &BillingHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
		strings.NewReader(`{"type":"checkout.session.completed"}`))
	req.Header.Set("Stripe-Signature", "v1,whatever")
	rr := httptest.NewRecorder()
	h.HandleWebhook(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503", rr.Code)
	}
}

func TestStripeWebhook_RejectsBadSignature(t *testing.T) {
	h := &BillingHandler{webhookSecret: "whsec_test"}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
		strings.NewReader(`{"type":"checkout.session.completed"}`))
	req.Header.Set("Stripe-Signature", "v1,not-a-real-signature")
	rr := httptest.NewRecorder()
	h.HandleWebhook(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestCreateCheckout_RejectsMissingPriceID(t *testing.T) {
	h := &BillingHandler{}
	cases := map[string]string{
		"empty_body": `{}`,
		"empty_str":  `{"price_id":""}`,
		"bad_json":   `not json`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/checkout",
				strings.NewReader(body))
			rr := httptest.NewRecorder()
			h.CreateCheckout(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
			}
			var resp map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp["code"] != "bad_request" {
				t.Errorf("code=%q want bad_request", resp["code"])
			}
		})
	}
}

// TestCreateCheckout_UnknownPlan — a plan-name the operator hasn't
// configured a STRIPE_PRICE_<PLAN> env var for must 400 with a
// distinct code; otherwise the frontend would silently fall back to
// some unrelated price and bill the customer the wrong amount.
func TestCreateCheckout_UnknownPlan(t *testing.T) {
	h := &BillingHandler{planToPrice: map[string]string{"starter": "price_starter"}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/checkout",
		strings.NewReader(`{"plan":"enterprise"}`))
	rr := httptest.NewRecorder()
	h.CreateCheckout(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["code"] != "unknown_plan" {
		t.Errorf("code=%q want unknown_plan", resp["code"])
	}
}

// checkoutCompletedPayload builds the raw JSON Stripe would send for a
// `checkout.session.completed` event carrying the metadata the handler
// reads to determine which tenant / plan to update. Subscription +
// customer IDs let the handler propagate both Stripe identifiers into
// the row in one shot.
func checkoutCompletedPayload(tenantID uuid.UUID, plan, subscriptionID, customerID string) string {
	return fmt.Sprintf(`{
		"id": "evt_test_%s",
		"type": "checkout.session.completed",
		"data": {
			"object": {
				"id": "cs_test_123",
				"object": "checkout.session",
				"customer": "%s",
				"subscription": "%s",
				"metadata": {"tenant_id": "%s", "plan": "%s"}
			}
		}
	}`, plan, customerID, subscriptionID, tenantID.String(), plan)
}

// TestStripeWebhook_CheckoutCompleted_UpdatesPlan is the tracer-bullet:
// a fully-signed checkout.session.completed event flows through
// HandleWebhook end-to-end and lands a LinkCheckoutSubscription call
// on the store with the right plan + limits + stripe IDs. The whole
// AC #4 ("the tenant's subscriptions.plan and quota_leads_per_month
// are updated within the webhook handler") rides on this path.
func TestStripeWebhook_CheckoutCompleted_UpdatesPlan(t *testing.T) {
	tenantID := uuid.New()
	const secret = "whsec_test_starter"
	fake := &fakeBillingQueries{}

	h := &BillingHandler{queries: fake, webhookSecret: secret}

	payload := checkoutCompletedPayload(tenantID, "starter", "sub_starter_1", "cus_test_1")
	sig := signStripePayload(t, payload, secret)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
		strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", sig)
	rr := httptest.NewRecorder()

	h.HandleWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s want 200", rr.Code, rr.Body.String())
	}
	if got := len(fake.linkCalls); got != 1 {
		t.Fatalf("LinkCheckoutSubscription calls=%d want 1", got)
	}
	got := fake.linkCalls[0]
	wantTenant := pgtype.UUID{Bytes: tenantID, Valid: true}
	if got.TenantID != wantTenant {
		t.Errorf("tenant_id=%v want %v", got.TenantID, wantTenant)
	}
	if got.Plan != "starter" {
		t.Errorf("plan=%q want starter", got.Plan)
	}
	if got.LeadsLimit != PlanLimits["starter"].Leads {
		t.Errorf("leads_limit=%d want %d", got.LeadsLimit, PlanLimits["starter"].Leads)
	}
	if got.SequencesLimit != PlanLimits["starter"].Sequences {
		t.Errorf("sequences_limit=%d want %d", got.SequencesLimit, PlanLimits["starter"].Sequences)
	}
	if !got.StripeCustomerID.Valid || got.StripeCustomerID.String != "cus_test_1" {
		t.Errorf("stripe_customer_id=%v want cus_test_1", got.StripeCustomerID)
	}
	if !got.StripeSubscriptionID.Valid || got.StripeSubscriptionID.String != "sub_starter_1" {
		t.Errorf("stripe_subscription_id=%v want sub_starter_1", got.StripeSubscriptionID)
	}
}

// TestStripeWebhook_CheckoutCompleted_Idempotent — Stripe retries
// undelivered events for up to a week. Replaying the same event must
// produce the same target row state, not double-apply or 5xx.
//
// Since LinkCheckoutSubscription is a stateless UPDATE, both replays
// land identical params; if we ever add incremental side-effects (e.g.
// audit log, counter), this test would need a dedupe check.
func TestStripeWebhook_CheckoutCompleted_Idempotent(t *testing.T) {
	tenantID := uuid.New()
	const secret = "whsec_test_idem"
	fake := &fakeBillingQueries{}

	h := &BillingHandler{queries: fake, webhookSecret: secret}

	payload := checkoutCompletedPayload(tenantID, "growth", "sub_growth_1", "cus_test_2")

	for i := 0; i < 2; i++ {
		sig := signStripePayload(t, payload, secret)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
			strings.NewReader(payload))
		req.Header.Set("Stripe-Signature", sig)
		rr := httptest.NewRecorder()
		h.HandleWebhook(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("replay %d: status=%d body=%s", i, rr.Code, rr.Body.String())
		}
	}

	if got := len(fake.linkCalls); got != 2 {
		t.Fatalf("LinkCheckoutSubscription calls=%d want 2 (one per replay)", got)
	}
	// Both calls must target the same row state — same plan, limits,
	// stripe IDs. A divergence would mean we leaked some kind of
	// monotonic state into the parameters.
	if fake.linkCalls[0] != fake.linkCalls[1] {
		t.Errorf("call params differ across replays: a=%+v b=%+v",
			fake.linkCalls[0], fake.linkCalls[1])
	}
}

// TestStripeWebhook_IrrelevantEvent_LoggedAnd200 — refund / dispute
// events arrive at this endpoint because Stripe sends every enabled
// event type to every webhook. We must not 4xx them (Stripe would
// retry indefinitely), must not call any state-changing queries, and
// must log enough to track them down later.
func TestStripeWebhook_IrrelevantEvent_LoggedAnd200(t *testing.T) {
	const secret = "whsec_test_other"
	fake := &fakeBillingQueries{}
	h := &BillingHandler{queries: fake, webhookSecret: secret}

	var logged strings.Builder
	resetLog := captureLog(t, &logged)
	defer resetLog()

	cases := []string{"charge.refunded", "charge.dispute.created"}
	for _, eventType := range cases {
		t.Run(eventType, func(t *testing.T) {
			payload := fmt.Sprintf(`{"id":"evt_%s","type":%q,"data":{"object":{}}}`,
				strings.ReplaceAll(eventType, ".", "_"), eventType)
			sig := signStripePayload(t, payload, secret)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
				strings.NewReader(payload))
			req.Header.Set("Stripe-Signature", sig)
			rr := httptest.NewRecorder()
			h.HandleWebhook(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("%s status=%d want 200 body=%s",
					eventType, rr.Code, rr.Body.String())
			}
		})
	}

	if len(fake.linkCalls) != 0 || len(fake.updateCustCalls) != 0 ||
		len(fake.updateByStripe) != 0 || len(fake.downgradeCalls) != 0 {
		t.Errorf("irrelevant events triggered DB calls: link=%d cust=%d byStripe=%d downgrade=%d",
			len(fake.linkCalls), len(fake.updateCustCalls),
			len(fake.updateByStripe), len(fake.downgradeCalls))
	}

	out := logged.String()
	for _, eventType := range cases {
		if !strings.Contains(out, eventType) {
			t.Errorf("log output missing event type %q; got:\n%s", eventType, out)
		}
	}
}
