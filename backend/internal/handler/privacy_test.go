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

func TestErasure_BadJSON(t *testing.T) {
	h := &PrivacyHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure",
		strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.Erasure(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

func TestErasure_NoIdentifier(t *testing.T) {
	h := &PrivacyHandler{}
	cases := map[string]string{
		"all_empty":   `{}`,
		"only_name":   `{"name":"Tim Cook"}`,    // name w/o company
		"only_company": `{"company":"Apple Inc"}`, // company w/o name
		"whitespace":  `{"email":"   ","linkedin_url":""}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/erasure",
				strings.NewReader(body))
			rr := httptest.NewRecorder()
			h.Erasure(rr, req)
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
