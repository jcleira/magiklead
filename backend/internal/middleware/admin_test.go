package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// AdminAuth's allowlist normalization runs on the constructed
// middleware before any handler call. The gate short-circuits on
// empty-allowlist, missing clerk-id, missing email, and non-match
// before invoking the downstream handler.

func TestAdminAuth_NoAllowlist(t *testing.T) {
	h := AdminAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	h := AdminAuth([]string{"admin@example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestAdminAuth_MissingEmail(t *testing.T) {
	h := AdminAuth([]string{"admin@example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClerkIDKey, "user_123"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "email claim") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestAdminAuth_EmailNotInAllowlist(t *testing.T) {
	h := AdminAuth([]string{"admin@example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx := context.WithValue(req.Context(), UserClerkIDKey, "user_123")
	ctx = context.WithValue(ctx, UserEmailKey, "other@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "required") {
		t.Errorf("body=%s", rr.Body.String())
	}
}

func TestAdminAuth_AllowlistMatch(t *testing.T) {
	called := false
	h := AdminAuth([]string{"Admin@Example.com "})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx := context.WithValue(req.Context(), UserClerkIDKey, "user_123")
	// ClerkAuth lowercases emails before stashing.
	ctx = context.WithValue(ctx, UserEmailKey, "admin@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !called {
		t.Fatal("downstream handler should have been called")
	}
}
