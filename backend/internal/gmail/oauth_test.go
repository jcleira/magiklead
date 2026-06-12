package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRevokeToken_Success — the revoke helper POSTs to Google's
// revoke endpoint with the token in the query string and treats a 200
// as success. Used by the settings page's disconnect Gmail flow.
func TestRevokeToken_Success(t *testing.T) {
	var gotToken string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotToken = r.URL.Query().Get("token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := RevokeToken(context.Background(), srv.URL, "ya29.test-access-token")
	if err != nil {
		t.Fatalf("RevokeToken err=%v want nil", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method=%q want POST", gotMethod)
	}
	if gotToken != "ya29.test-access-token" {
		t.Errorf("token=%q want ya29.test-access-token", gotToken)
	}
}

// TestRevokeToken_400AlreadyRevoked — Google returns 400 with
// "invalid_token" when the token has already been revoked or expired.
// That's not an error from the caller's perspective: the goal
// (the token can no longer be used to send mail) is already met.
// Surfacing it as success lets the settings DELETE handler proceed to
// the row delete instead of leaving the user with a row they can't
// remove.
func TestRevokeToken_400AlreadyRevoked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer srv.Close()

	err := RevokeToken(context.Background(), srv.URL, "expired-token")
	if err != nil {
		t.Errorf("RevokeToken on already-revoked token err=%v want nil", err)
	}
}

// TestRevokeToken_5xxFailure — a 5xx from Google IS an error worth
// surfacing: the token may still be live, and we don't want to delete
// the local row while the OAuth grant is still valid on Google's side.
func TestRevokeToken_5xxFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	err := RevokeToken(context.Background(), srv.URL, "ya29.test")
	if err == nil {
		t.Error("RevokeToken on 5xx err=nil, want error")
	}
}

// TestRevokeToken_EmptyToken — defensive: no token, no request, no
// noise. The settings DELETE handler may encounter rows with empty
// tokens (legacy / partial inserts); revoking nothing should be a
// silent no-op rather than an attempt to call Google with token="".
func TestRevokeToken_EmptyToken(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := RevokeToken(context.Background(), srv.URL, ""); err != nil {
		t.Errorf("RevokeToken with empty token err=%v want nil", err)
	}
	if called {
		t.Error("RevokeToken called Google with empty token; want short-circuit")
	}
}
