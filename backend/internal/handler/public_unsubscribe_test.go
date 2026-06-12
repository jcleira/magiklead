package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/suppression"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// fakeRecorder is a hand-rolled stand-in for suppression.Module's
// RecordUnsubscribe — keeps the handler tests free of a real DB while
// still letting us assert exactly which (email, tenant, reason)
// triple the handler asked us to write.
type fakeRecorder struct {
	mu     sync.Mutex
	calls  []recordedUnsub
	errOut error
}

type recordedUnsub struct {
	Email    string
	TenantID *uuid.UUID
	Reason   string
}

func (f *fakeRecorder) RecordUnsubscribe(_ context.Context, email string, tenantID *uuid.UUID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedUnsub{Email: email, TenantID: tenantID, Reason: reason})
	return f.errOut
}

var unsubSecret = []byte("public-unsubscribe-test-secret")

func validUnsubToken(t *testing.T, email string, tenantID uuid.UUID) string {
	t.Helper()
	tok, err := jwt.Encode(jwt.Claims{
		Email:    email,
		TenantID: tenantID.String(),
		Exp:      time.Now().Add(time.Hour).Unix(),
	}, unsubSecret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return tok
}

// Tracer: a valid POST flows through JWT decode → RecordUnsubscribe →
// 200 HTML confirmation. Proves the full happy path is wired before
// drilling into error branches.
func TestPublicUnsubscribe_PostValid(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	email := "lead@example.com"
	tenantID := uuid.New()
	tok := validUnsubToken(t, email, tenantID)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe?token="+tok, nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type=%q want text/html...", ct)
	}
	if !strings.Contains(rr.Body.String(), "unsubscribed") {
		t.Errorf("body=%q want a confirmation containing 'unsubscribed'", rr.Body.String())
	}

	if len(rec.calls) != 1 {
		t.Fatalf("recorder calls=%d want 1", len(rec.calls))
	}
	got := rec.calls[0]
	if got.Email != email {
		t.Errorf("email=%q want %q", got.Email, email)
	}
	if got.TenantID == nil || *got.TenantID != tenantID {
		t.Errorf("tenantID=%v want %v", got.TenantID, tenantID)
	}
	if got.Reason != suppression.ReasonListUnsubscribe {
		t.Errorf("reason=%q want %q", got.Reason, suppression.ReasonListUnsubscribe)
	}
}

// GET path matches POST: some clients pre-fetch links on hover.
// RFC 8058 keeps POST as the one-click action, but GET is a
// legitimate fallback and must return the same confirmation.
func TestPublicUnsubscribe_GetValid(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	tenantID := uuid.New()
	tok := validUnsubToken(t, "a@b.com", tenantID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/unsubscribe?token="+tok, nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200, body=%s", rr.Code, rr.Body.String())
	}
	if len(rec.calls) != 1 {
		t.Errorf("recorder calls=%d want 1", len(rec.calls))
	}
}

func TestPublicUnsubscribe_ExpiredToken(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	tok, err := jwt.Encode(jwt.Claims{
		Email:    "a@b.com",
		TenantID: uuid.NewString(),
		Exp:      time.Now().Add(-1 * time.Minute).Unix(),
	}, unsubSecret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe?token="+tok, nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusGone {
		t.Errorf("status=%d want 410", rr.Code)
	}
	if len(rec.calls) != 0 {
		t.Errorf("recorder called on expired token (calls=%d)", len(rec.calls))
	}
}

func TestPublicUnsubscribe_TamperedToken(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	// Token signed with a different secret.
	tok, err := jwt.Encode(jwt.Claims{
		Email:    "a@b.com",
		TenantID: uuid.NewString(),
		Exp:      time.Now().Add(time.Hour).Unix(),
	}, []byte("wrong-secret"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe?token="+tok, nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rr.Code)
	}
	if len(rec.calls) != 0 {
		t.Errorf("recorder called on tampered token (calls=%d)", len(rec.calls))
	}
}

// Missing or empty token lands in the same 401 bucket as tampered —
// either way, the request can't be trusted.
func TestPublicUnsubscribe_MissingToken(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe", nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rr.Code)
	}
}

// Idempotent: calling the handler twice with the same token returns
// 200 both times, with no requirement for the recorder to deduplicate.
// The handler trusts the DB's ON CONFLICT DO NOTHING; the contract
// here is that the public response stays 200 and never leaks the fact
// that the address was already on the list.
func TestPublicUnsubscribe_Idempotent(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	tenantID := uuid.New()
	tok := validUnsubToken(t, "a@b.com", tenantID)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe?token="+tok, nil)
		rr := httptest.NewRecorder()
		h.Handle(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("iteration %d status=%d want 200", i, rr.Code)
		}
	}
	if len(rec.calls) != 2 {
		t.Errorf("recorder calls=%d want 2 (handler doesn't dedupe; DB does)", len(rec.calls))
	}
}

// A token whose tenant_id claim isn't a valid UUID can't have been
// issued by BuildUnsubscribeHeaders — treat as tampered.
func TestPublicUnsubscribe_InvalidTenantClaim(t *testing.T) {
	rec := &fakeRecorder{}
	h := NewPublicUnsubscribeHandler(unsubSecret, rec)

	tok, err := jwt.Encode(jwt.Claims{
		Email:    "a@b.com",
		TenantID: "not-a-uuid",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}, unsubSecret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/unsubscribe?token="+tok, nil)
	rr := httptest.NewRecorder()
	h.Handle(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rr.Code)
	}
	if len(rec.calls) != 0 {
		t.Errorf("recorder called on invalid tenant claim (calls=%d)", len(rec.calls))
	}
}
