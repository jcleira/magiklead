package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Validation paths run before any DB call, so we can hit them with a
// nil-queries handler.

func TestRequestErasure_BadJSON(t *testing.T) {
	h := &PrivacyHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure/request",
		strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.RequestErasure(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestRequestErasure_NoIdentifier(t *testing.T) {
	h := &PrivacyHandler{}
	cases := map[string]string{
		"all_empty":    `{}`,
		"only_name":    `{"name":"Tim Cook"}`,
		"only_company": `{"company":"Apple Inc"}`,
		"whitespace":   `{"email":"   ","linkedin_url":""}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure/request",
				strings.NewReader(body))
			rr := httptest.NewRecorder()
			h.RequestErasure(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
			}
			var resp map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if resp["code"] != "no_identifier" {
				t.Errorf("code=%q want no_identifier", resp["code"])
			}
		})
	}
}

func TestRequestErasure_EmailRequired(t *testing.T) {
	h := &PrivacyHandler{}
	// A linkedin-only request identifies a person but leaves us with
	// no address to deliver the confirmation link.
	body := `{"linkedin_url":"https://www.linkedin.com/in/timcook"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure/request",
		strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.RequestErasure(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["code"] != "email_required" {
		t.Errorf("code=%q want email_required", resp["code"])
	}
}

func TestConfirmErasure_MissingToken(t *testing.T) {
	h := &PrivacyHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure/confirm", nil)
	rr := httptest.NewRecorder()
	h.ConfirmErasure(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["code"] != "token_required" {
		t.Errorf("code=%q want token_required", resp["code"])
	}
}
