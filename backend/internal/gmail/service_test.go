package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

// TestFetchEmail_ReturnsRealMailbox — the connect callback stores the
// From address from Google userinfo, not a placeholder. FetchEmail must
// send the access token as a bearer and parse the `email` field.
func TestFetchEmail_ReturnsRealMailbox(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"123","email":"operator@realmailbox.com","verified_email":true}`))
	}))
	defer srv.Close()

	s := &Service{userinfoEndpoint: srv.URL}
	email, err := s.FetchEmail(context.Background(), &oauth2.Token{AccessToken: "ya29.real"})
	if err != nil {
		t.Fatalf("FetchEmail: %v", err)
	}
	if email != "operator@realmailbox.com" {
		t.Errorf("email=%q want operator@realmailbox.com", email)
	}
	if gotAuth != "Bearer ya29.real" {
		t.Errorf("Authorization=%q want %q", gotAuth, "Bearer ya29.real")
	}
}

// TestFetchEmail_Non200 — a non-200 from userinfo is surfaced as an
// error so the callback redirects to the error page rather than saving
// an account with a blank From address.
func TestFetchEmail_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	s := &Service{userinfoEndpoint: srv.URL}
	if _, err := s.FetchEmail(context.Background(), &oauth2.Token{AccessToken: "ya29.expired"}); err == nil {
		t.Error("expected an error on non-200 userinfo response")
	}
}

// TestFetchEmail_EmptyEmail — a 200 that carries no email is an error;
// saving an empty From address would break every send.
func TestFetchEmail_EmptyEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"123"}`))
	}))
	defer srv.Close()

	s := &Service{userinfoEndpoint: srv.URL}
	if _, err := s.FetchEmail(context.Background(), &oauth2.Token{AccessToken: "ya29.x"}); err == nil {
		t.Error("expected an error when userinfo carries no email")
	}
}
