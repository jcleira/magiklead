package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
