package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/bootstrap"
)

// EnsureTenant: the contract is "guarantee a tenant ID in context or
// reject". On a JWT with no clerk ID it short-circuits (auth layer
// would have returned 401 already, but the middleware should not
// crash). On a present clerk ID it consults the lookup callback; if
// the lookup returns a tenant, that tenant lands in context. If the
// lookup returns "no user", the middleware calls bootstrap; if
// bootstrap returns ErrNoEmail it surfaces 503 with a body that names
// the JWT template, otherwise 5xx for any other error.

func TestEnsureTenant_NoClerkID(t *testing.T) {
	called := false
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		t.Fatal("lookup should not run when no clerk ID is in context")
		return uuid.Nil, false, nil
	}
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		t.Fatal("bootstrap should not run")
		return uuid.Nil, nil
	}
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Context().Value(TenantIDKey) != nil {
			t.Error("tenant ID should not be set when clerk ID is missing")
		}
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !called {
		t.Fatal("downstream handler should still run (auth middleware will reject upstream)")
	}
}

func TestEnsureTenant_ExistingUser(t *testing.T) {
	wantTenant := uuid.New()
	lookupCalls := 0
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		lookupCalls++
		if clerkID != "user_exists" {
			t.Errorf("lookup got clerkID=%q want user_exists", clerkID)
		}
		return wantTenant, true, nil
	}
	bootCalls := 0
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		bootCalls++
		return uuid.Nil, nil
	}
	called := false
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotTenant, ok := r.Context().Value(TenantIDKey).(uuid.UUID)
		if !ok || gotTenant != wantTenant {
			t.Errorf("tenant ID in ctx=%v want %v", gotTenant, wantTenant)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClerkIDKey, "user_exists"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("downstream handler should have been called")
	}
	if lookupCalls != 1 {
		t.Errorf("lookupCalls=%d want 1", lookupCalls)
	}
	if bootCalls != 0 {
		t.Errorf("bootstrap was called %d times for an existing user; want 0", bootCalls)
	}
}

func TestEnsureTenant_MissingUserBootstraps(t *testing.T) {
	wantTenant := uuid.New()
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		return uuid.Nil, false, nil // no user
	}
	bootCalls := 0
	var capturedClerkID, capturedEmail, capturedName string
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		bootCalls++
		capturedClerkID = clerkID
		capturedEmail = email
		capturedName = name
		return wantTenant, nil
	}
	called := false
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotTenant, ok := r.Context().Value(TenantIDKey).(uuid.UUID)
		if !ok || gotTenant != wantTenant {
			t.Errorf("tenant ID in ctx=%v want %v", gotTenant, wantTenant)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx := context.WithValue(req.Context(), UserClerkIDKey, "user_new")
	ctx = context.WithValue(ctx, UserEmailKey, "new@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called {
		t.Fatal("downstream handler should have been called")
	}
	if bootCalls != 1 {
		t.Errorf("bootCalls=%d want 1", bootCalls)
	}
	if capturedClerkID != "user_new" {
		t.Errorf("bootstrap clerkID=%q want user_new", capturedClerkID)
	}
	if capturedEmail != "new@example.com" {
		t.Errorf("bootstrap email=%q want new@example.com", capturedEmail)
	}
	if capturedName != "" {
		t.Errorf("bootstrap name=%q want empty (no JWT name claim today)", capturedName)
	}
}

func TestEnsureTenant_ErrNoEmail_503WithJWTHint(t *testing.T) {
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		return uuid.Nil, false, nil
	}
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		return uuid.Nil, bootstrap.ErrNoEmail
	}
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler should not run when bootstrap fails")
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClerkIDKey, "user_no_email"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503", rr.Code)
	}
	body := rr.Body.String()
	// The body must point the developer at the JWT-template
	// configuration so they fix it instead of chasing a downstream
	// NULL.
	if !strings.Contains(strings.ToLower(body), "jwt") || !strings.Contains(strings.ToLower(body), "template") {
		t.Errorf("body=%q does not name the JWT template as the cause", body)
	}
}

func TestEnsureTenant_GenericBootstrapError_500(t *testing.T) {
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		return uuid.Nil, false, nil
	}
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		return uuid.Nil, errors.New("postgres exploded")
	}
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler should not run when bootstrap fails")
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx := context.WithValue(req.Context(), UserClerkIDKey, "user_x")
	ctx = context.WithValue(ctx, UserEmailKey, "x@example.com")
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", rr.Code)
	}
}

func TestEnsureTenant_LookupError_500(t *testing.T) {
	lookup := func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		return uuid.Nil, false, errors.New("connection reset")
	}
	bootCalls := 0
	boot := func(ctx context.Context, clerkID, email, name string) (uuid.UUID, error) {
		bootCalls++
		return uuid.Nil, nil
	}
	h := EnsureTenant(lookup, boot)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler should not run when lookup fails")
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserClerkIDKey, "user_x"))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", rr.Code)
	}
	if bootCalls != 0 {
		t.Errorf("bootstrap called %d times after lookup error; want 0 (don't paper over an unrelated DB failure)", bootCalls)
	}
}
