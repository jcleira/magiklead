package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AdminAuth's allowlist normalization runs on the constructed
// middleware before any handler call. Verify it's case-insensitive
// and trims whitespace by hitting the gate without configuring queries
// — the empty-allowlist branch and the missing-clerk-id branch both
// short-circuit before the DB lookup.

func TestAdminAuth_NoAllowlist(t *testing.T) {
	h := AdminAuth(nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "not configured") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestAdminAuth_MissingClerkID(t *testing.T) {
	h := AdminAuth(nil, []string{"admin@example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d", rr.Code)
	}
}
