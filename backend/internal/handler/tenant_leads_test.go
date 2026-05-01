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
)

// All tests in this file run on a nil-queries handler — the
// validation paths under test reject the request before any DB call.

func TestTenantLeadAdd_BadJSON(t *testing.T) {
	h := &TenantLeadHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant_leads",
		strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.Add(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestTenantLeadAdd_InvalidPersonID(t *testing.T) {
	h := &TenantLeadHandler{}
	cases := map[string]string{
		"missing":   `{}`,
		"malformed": `{"person_id":"not-a-uuid"}`,
		"empty":     `{"person_id":"  "}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant_leads",
				strings.NewReader(body))
			rr := httptest.NewRecorder()
			h.Add(rr, req)
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

func TestTenantLeadUpdate_BadJSON(t *testing.T) {
	h := &TenantLeadHandler{}
	req := newChiRequest(t, http.MethodPatch, "/api/v1/tenant_leads/{person_id}",
		uuid.NewString(), strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestTenantLeadUpdate_NoFields(t *testing.T) {
	// Empty patch — neither status nor notes — must 400 before
	// touching the DB. The "no-op update" semantic would silently
	// confuse callers.
	h := &TenantLeadHandler{}
	req := newChiRequest(t, http.MethodPatch, "/api/v1/tenant_leads/{person_id}",
		uuid.NewString(), strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(resp["message"], "status or notes") {
		t.Errorf("message=%q want message about status/notes", resp["message"])
	}
}

func TestTenantLeadUpdate_InvalidPersonID(t *testing.T) {
	h := &TenantLeadHandler{}
	req := newChiRequest(t, http.MethodPatch, "/api/v1/tenant_leads/{person_id}",
		"not-a-uuid", strings.NewReader(`{"status":"new"}`))
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestTenantLeadDelete_InvalidPersonID(t *testing.T) {
	h := &TenantLeadHandler{}
	req := newChiRequest(t, http.MethodDelete, "/api/v1/tenant_leads/{person_id}",
		"not-a-uuid", nil)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

// newChiRequest builds an http.Request with the chi URL param set so
// `chi.URLParam(r, "person_id")` returns `paramValue`. Used by tests
// that target handlers expecting a `{person_id}` route segment.
func newChiRequest(t *testing.T, method, pattern, paramValue string, body interface{ Read([]byte) (int, error) }) *http.Request {
	t.Helper()
	url := strings.Replace(pattern, "{person_id}", paramValue, 1)
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, url, body)
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("person_id", paramValue)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req
}

// Compile-time assertion that the middleware context key is exported
// for tests that need to inject tenant/user context. Catches a future
// refactor that accidentally hides it.
var _ = middleware.UserClerkIDKey
