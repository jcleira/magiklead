package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// nonNilSupp returns a real suppression.Module pointer that survives
// the handler's nil-check guard but would crash if any DB-touching
// method were invoked. The validation paths under test return
// before the handler reaches a supp method, so the constructor's
// nil pool never gets dereferenced.
func nonNilSupp() *suppression.Module {
	return suppression.New(nil)
}

// withURLParams injects chi's URL-param context onto a request so the
// handler's chi.URLParam calls resolve. Without this, calls return
// the empty string and every UUID parse rejects with 400 — the test
// would never reach the cases it's trying to exercise.
func withURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// withTenant injects a tenant id into the request context the way
// middleware.EnsureTenant does in production. getTenantID reads
// from this key.
func withTenant(req *http.Request, tenantID uuid.UUID) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
}

// Reengage validation paths run before any DB call, so a handler with
// nil queries / nil supp is enough to exercise them. The tenant-
// scoping and ClearReply paths are exercised by Playwright in #14
// against a live devpod.

func TestReengage_MissingSuppression(t *testing.T) {
	// supp == nil → 503; this is the guard the constructor leaves in
	// place when callers haven't wired the module.
	h := &CampaignHandler{}
	req := httptest.NewRequest(http.MethodPost, "/r", nil)
	req = withTenant(req, uuid.New())
	rr := httptest.NewRecorder()
	h.Reengage(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503", rr.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["code"] != "suppression_unavailable" {
		t.Errorf("code=%q want suppression_unavailable", resp["code"])
	}
}

func TestReengage_BadCampaignID(t *testing.T) {
	// Stand-in non-nil supp so the 503 guard doesn't short-circuit
	// us out of the validation path we want to test.
	h := &CampaignHandler{supp: nonNilSupp()}
	req := httptest.NewRequest(http.MethodPost, "/r", nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": "not-a-uuid", "lead_id": uuid.NewString()})
	rr := httptest.NewRecorder()
	h.Reengage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid campaign id") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestReengage_BadLeadID(t *testing.T) {
	h := &CampaignHandler{supp: nonNilSupp()}
	req := httptest.NewRequest(http.MethodPost, "/r", nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": uuid.NewString(), "lead_id": "also-not-a-uuid"})
	rr := httptest.NewRecorder()
	h.Reengage(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid lead id") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

// Create validates the LinkedIn authoring contract before touching the
// DB, so a handler with nil queries is enough to exercise it. A step-0
// connection note over LinkedIn's 300-char cap must 4xx rather than
// persist an unsendable note.
func TestCreate_LinkedInNoteTooLong(t *testing.T) {
	h := &CampaignHandler{} // validation returns before any DB call
	body, _ := json.Marshal(map[string]any{
		"play_id": uuid.NewString(),
		"name":    "LI camp",
		"channel": "linkedin",
		"linkedin_sequence": []map[string]any{
			{"step": 0, "delay_days": 0, "body": strings.Repeat("a", 301)},
			{"step": 1, "delay_days": 2, "body": "hello {{first_name}}"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/campaigns", strings.NewReader(string(body)))
	req = withTenant(req, uuid.New())
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// A LinkedIn campaign with only a connection note and no DM step is an
// incomplete sequence — the contract requires step-0 plus ≥1 DM.
func TestCreate_LinkedInRequiresDMStep(t *testing.T) {
	h := &CampaignHandler{}
	body, _ := json.Marshal(map[string]any{
		"play_id": uuid.NewString(),
		"name":    "LI camp",
		"channel": "linkedin",
		"linkedin_sequence": []map[string]any{
			{"step": 0, "delay_days": 0, "body": "hi, let's connect"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/campaigns", strings.NewReader(string(body)))
	req = withTenant(req, uuid.New())
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// Metrics is gated on a valid UUID before any DB access — verifiable
// without a live Postgres. Behaviour against the database is exercised
// by campaigns_metrics_test.go under the integration build tag.
func TestMetrics_BadCampaignID(t *testing.T) {
	h := &CampaignHandler{}
	req := httptest.NewRequest(http.MethodGet, "/m", nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": "not-a-uuid"})
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}
