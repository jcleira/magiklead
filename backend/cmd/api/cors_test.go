package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/cors"
)

// preflight sends the CORS preflight of a browser at origin to a handler
// with the api's CORS options, and returns the allowed origin it answers.
func preflight(t *testing.T, opts cors.Options, origin string) string {
	t.Helper()
	h := cors.Handler(opts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/me", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Header().Get("Access-Control-Allow-Origin")
}

// A devpod answers at two names, so the browser can call the api from two
// origins: FRONTEND_URL and each origin of the CORS_EXTRA_ORIGINS list.
func TestCORSOptionsAllowTwoOrigins(t *testing.T) {
	opts := corsOptions("https://app.magiklead.z13.internal", " https://app.magiklead.localhost ,")

	for _, origin := range []string{"https://app.magiklead.z13.internal", "https://app.magiklead.localhost"} {
		if got := preflight(t, opts, origin); got != origin {
			t.Errorf("preflight from %s: Access-Control-Allow-Origin = %q, want %q", origin, got, origin)
		}
	}
	if got := preflight(t, opts, "https://evil.example.com"); got != "" {
		t.Errorf("preflight from another origin: Access-Control-Allow-Origin = %q, want none", got)
	}
}

// With no extra origins (production), only FRONTEND_URL is allowed.
func TestCORSOptionsWithNoExtraOrigins(t *testing.T) {
	opts := corsOptions("https://app.magiklead.com", "")
	if len(opts.AllowedOrigins) != 1 || opts.AllowedOrigins[0] != "https://app.magiklead.com" {
		t.Errorf("AllowedOrigins = %q, want only FRONTEND_URL", opts.AllowedOrigins)
	}
}
