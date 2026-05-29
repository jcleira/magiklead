package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// fakeGmailQueries stubs the gmailHandlerQueries methods the
// DeleteAccount flow exercises. The handler must call GetGmailAccount
// first (to get the token to revoke), then DeleteGmailAccount; the
// fake captures both calls so the test can assert ordering and
// arguments.
type fakeGmailQueries struct {
	user               repository.User
	userErr            error
	account            repository.GmailAccount
	getErr             error
	deleteErr          error
	getCalledWith      repository.GetGmailAccountParams
	deleteCalledWith   repository.DeleteGmailAccountParams
	getCalled          bool
	deleteCalled       bool
	deleteCalledBefore bool // set if delete was called before get — bug
	revokeCalledBefore bool // set by the httptest handler before delete
	deleteCalledAfter  bool // set by delete if revokeCalledBefore was true
}

func (f *fakeGmailQueries) GetGmailAccount(ctx context.Context, arg repository.GetGmailAccountParams) (repository.GmailAccount, error) {
	if f.deleteCalled {
		f.deleteCalledBefore = true
	}
	f.getCalled = true
	f.getCalledWith = arg
	return f.account, f.getErr
}

func (f *fakeGmailQueries) GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error) {
	return f.user, f.userErr
}

func (f *fakeGmailQueries) ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error) {
	return nil, nil
}

func (f *fakeGmailQueries) DeleteGmailAccount(ctx context.Context, arg repository.DeleteGmailAccountParams) error {
	if f.revokeCalledBefore {
		f.deleteCalledAfter = true
	}
	f.deleteCalled = true
	f.deleteCalledWith = arg
	return f.deleteErr
}

// TestGmail_DeleteAccount_RevokesThenDeletes is the wiring test the
// settings disconnect flow depends on. Order matters: revoke first
// (so a Google outage doesn't leave a still-live grant behind a
// deleted local row), then delete.
func TestGmail_DeleteAccount_RevokesThenDeletes(t *testing.T) {
	accountID := uuid.New()
	tenantID := uuid.New()
	token := "ya29.connected-mailbox-token"

	fake := &fakeGmailQueries{
		account: repository.GmailAccount{
			ID:          pgtype.UUID{Bytes: accountID, Valid: true},
			TenantID:    pgtype.UUID{Bytes: tenantID, Valid: true},
			Email:       "operator@example.com",
			AccessToken: token,
		},
	}

	// Revoke stub: capture the call, then flip the fake's flag so the
	// downstream DeleteGmailAccount records that revoke ran first.
	var revokedWith string
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		revokedWith = r.URL.Query().Get("token")
		fake.revokeCalledBefore = true
		w.WriteHeader(http.StatusOK)
	}))
	defer revokeSrv.Close()

	h := &GmailHandler{queries: fake, revokeEndpoint: revokeSrv.URL}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gmail/accounts/"+accountID.String(), nil)
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": accountID.String()})
	rr := httptest.NewRecorder()

	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if !fake.getCalled {
		t.Error("GetGmailAccount was not called")
	}
	if revokedWith != token {
		t.Errorf("revoke called with token=%q want %q", revokedWith, token)
	}
	if !fake.deleteCalled {
		t.Error("DeleteGmailAccount was not called")
	}
	if fake.deleteCalledBefore {
		t.Error("DeleteGmailAccount ran before GetGmailAccount — wrong order")
	}
	if !fake.deleteCalledAfter {
		t.Error("DeleteGmailAccount must run AFTER the revoke succeeds")
	}
	// Tenant scoping: both queries must carry the tenant id from
	// context, never the request body or URL.
	wantPG := pgtype.UUID{Bytes: tenantID, Valid: true}
	if fake.getCalledWith.TenantID != wantPG {
		t.Errorf("Get tenant=%v want %v — possible cross-tenant read", fake.getCalledWith.TenantID, wantPG)
	}
	if fake.deleteCalledWith.TenantID != wantPG {
		t.Errorf("Delete tenant=%v want %v — possible cross-tenant write", fake.deleteCalledWith.TenantID, wantPG)
	}
}

// TestGmail_DeleteAccount_RevokeFailureStillDeletes — if Google is
// briefly unreachable, the user's Disconnect click still removes the
// local row. The grant can be cleaned up out-of-band; the UX must not
// dead-end. See the doc comment on DeleteAccount for the rationale.
func TestGmail_DeleteAccount_RevokeFailureStillDeletes(t *testing.T) {
	accountID := uuid.New()
	tenantID := uuid.New()
	fake := &fakeGmailQueries{
		account: repository.GmailAccount{
			ID:          pgtype.UUID{Bytes: accountID, Valid: true},
			AccessToken: "ya29.test",
		},
	}
	revokeSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // upstream failure
	}))
	defer revokeSrv.Close()

	h := &GmailHandler{queries: fake, revokeEndpoint: revokeSrv.URL}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gmail/accounts/"+accountID.String(), nil)
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": accountID.String()})
	rr := httptest.NewRecorder()

	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204 (revoke failure must not block delete)", rr.Code)
	}
	if !fake.deleteCalled {
		t.Error("DeleteGmailAccount must run even when revoke fails")
	}
}

// TestGmail_DeleteAccount_AccountNotFound — deleting an account that
// doesn't exist (or belongs to another tenant) still returns 204
// rather than 404, because the user's goal — "this mailbox should not
// send for me anymore" — is already met by the row's absence. 204 is
// the idempotent-delete contract.
func TestGmail_DeleteAccount_AccountNotFound(t *testing.T) {
	fake := &fakeGmailQueries{getErr: errors.New("no rows")}
	h := &GmailHandler{queries: fake, revokeEndpoint: "http://unreachable"}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gmail/accounts/"+uuid.NewString(), nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": uuid.NewString()})
	rr := httptest.NewRecorder()

	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204", rr.Code)
	}
	if fake.deleteCalled {
		t.Error("DeleteGmailAccount must not run when account doesn't exist")
	}
}

// TestGmail_DeleteAccount_BadID — a malformed UUID rejects before any
// DB call.
func TestGmail_DeleteAccount_BadID(t *testing.T) {
	h := &GmailHandler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gmail/accounts/not-a-uuid", nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": "not-a-uuid"})
	rr := httptest.NewRecorder()

	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rr.Code)
	}
}

// --- Connect flow (AuthURL + Callback) ---
//
// These exercise the real OAuth connect path the SQL-stub verification
// walks skipped: they were inserting gmail_accounts rows directly, so
// the four connect-path bugs (callback behind ClerkAuth, redirect URI,
// user_id=tenant_id placeholder, faked email) never surfaced. The fake
// service stands in for Google + the DB so the handler logic — signed
// state, the tenant/user binding, and the real email — is asserted
// hermetically.

type fakeGmailService struct {
	authURL       string
	exchangeToken *oauth2.Token
	exchangeErr   error
	fetchEmail    string
	fetchErr      error
	saveErr       error

	exchangeCalled bool
	fetchCalled    bool
	saveCalled     bool
	savedTenantID  pgtype.UUID
	savedUserID    pgtype.UUID
	savedEmail     string
}

func (f *fakeGmailService) GetAuthURL(state string) string {
	base := f.authURL
	if base == "" {
		base = "https://accounts.google.com/o/oauth2/auth"
	}
	return base + "?state=" + state
}

func (f *fakeGmailService) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	f.exchangeCalled = true
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	if f.exchangeToken != nil {
		return f.exchangeToken, nil
	}
	return &oauth2.Token{AccessToken: "ya29.fake"}, nil
}

func (f *fakeGmailService) FetchEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	f.fetchCalled = true
	if f.fetchErr != nil {
		return "", f.fetchErr
	}
	return f.fetchEmail, nil
}

func (f *fakeGmailService) SaveAccount(ctx context.Context, tenantID, userID pgtype.UUID, email string, token *oauth2.Token) (*repository.GmailAccount, error) {
	f.saveCalled = true
	f.savedTenantID = tenantID
	f.savedUserID = userID
	f.savedEmail = email
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return &repository.GmailAccount{TenantID: tenantID, UserID: userID, Email: email}, nil
}

// withUserClerkID injects a Clerk user id the way middleware.ClerkAuth
// does, so AuthURL can resolve the connecting user.
func withUserClerkID(req *http.Request, clerkID string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClerkIDKey, clerkID))
}

// signGmailState mints a state token the same way AuthURL does, so the
// Callback tests can drive a valid (or deliberately stale) state.
func signGmailState(t *testing.T, secret []byte, tenantID, userID uuid.UUID, exp time.Time) string {
	t.Helper()
	s, err := jwt.Encode(jwt.Claims{
		TenantID: tenantID.String(),
		UserID:   userID.String(),
		Exp:      exp.Unix(),
	}, secret)
	if err != nil {
		t.Fatalf("sign state: %v", err)
	}
	return s
}

// TestGmail_Callback_PublicSignedState_SavesRealUserAndEmail is the
// real-callback integration test. It drives Callback with NO auth in
// context (proving the route works publicly, bound only by the signed
// state) and asserts the three data-correctness bugs are fixed: the
// saved user_id is the real connecting user (not the tenant id), and
// the saved email is the address Google's userinfo returned (not the
// connected@gmail.com placeholder). It also asserts the response is a
// redirect, never JSON — the account carries OAuth tokens that must not
// reach the browser.
func TestGmail_Callback_PublicSignedState_SavesRealUserAndEmail(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	tenantID := uuid.New()
	userID := uuid.New() // the real connecting user, distinct from tenant
	realEmail := "operator@realmailbox.com"

	svc := &fakeGmailService{
		fetchEmail:    realEmail,
		exchangeToken: &oauth2.Token{AccessToken: "ya29.real", RefreshToken: "rt.real"},
	}
	h := &GmailHandler{gmailSvc: svc, stateSecret: secret, frontendURL: "https://app.example.com"}

	state := signGmailState(t, secret, tenantID, userID, time.Now().Add(10*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/callback?code=auth-code-xyz&state="+state, nil)
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status=%d want 302 (browser redirect) body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://app.example.com/settings") || !strings.Contains(loc, "gmail=connected") {
		t.Errorf("redirect Location=%q want frontend settings with gmail=connected", loc)
	}
	if strings.Contains(rr.Body.String(), "ya29.real") || strings.Contains(rr.Body.String(), "rt.real") {
		t.Error("OAuth tokens leaked into the callback response body")
	}
	if !svc.saveCalled {
		t.Fatal("SaveAccount was not called")
	}
	// Bug 3: user_id must be the real connecting user, never the tenant.
	if got := uuid.UUID(svc.savedUserID.Bytes); got != userID {
		t.Errorf("saved user_id=%s want %s — FK must point at the real user, not the tenant", got, userID)
	}
	if got := uuid.UUID(svc.savedTenantID.Bytes); got != tenantID {
		t.Errorf("saved tenant_id=%s want %s", got, tenantID)
	}
	if svc.savedUserID == svc.savedTenantID {
		t.Error("user_id == tenant_id — the placeholder FK bug is back")
	}
	// Bug 4: the stored From address comes from Google userinfo.
	if svc.savedEmail != realEmail {
		t.Errorf("saved email=%q want %q — must come from userinfo, not a placeholder", svc.savedEmail, realEmail)
	}
	if svc.savedEmail == "connected@gmail.com" {
		t.Error("email is the connected@gmail.com placeholder — the fake-email bug is back")
	}
}

// TestGmail_Callback_DeniedOrMissingCode_RedirectsError — when the user
// denies consent Google redirects back with the state but no code. The
// callback must send the browser back to settings with an error flag,
// not exchange or save anything.
func TestGmail_Callback_DeniedOrMissingCode_RedirectsError(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	state := signGmailState(t, secret, uuid.New(), uuid.New(), time.Now().Add(10*time.Minute))
	svc := &fakeGmailService{fetchEmail: "x@y.com"}
	h := &GmailHandler{gmailSvc: svc, stateSecret: secret, frontendURL: "https://app.example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/callback?state="+state, nil)
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusFound || !strings.Contains(rr.Header().Get("Location"), "gmail=error") {
		t.Errorf("status=%d loc=%q want 302 -> gmail=error", rr.Code, rr.Header().Get("Location"))
	}
	if svc.exchangeCalled || svc.saveCalled {
		t.Error("missing code must not trigger exchange or save")
	}
}

// TestGmail_Callback_ForgedState_NoExchangeNoSave — a state that isn't
// a token signed with our secret (e.g. the old random hex, or an
// attacker's value) must be rejected before any token exchange, so a
// forged tenant/user binding can never be persisted.
func TestGmail_Callback_ForgedState_NoExchangeNoSave(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	svc := &fakeGmailService{fetchEmail: "x@y.com"}
	h := &GmailHandler{gmailSvc: svc, stateSecret: secret, frontendURL: "https://app.example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/callback?code=c&state=deadbeefdeadbeefdeadbeef", nil)
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if svc.exchangeCalled {
		t.Error("code was exchanged before the state was validated")
	}
	if svc.saveCalled {
		t.Error("SaveAccount ran for a forged/unsigned state — the binding is not enforced")
	}
	if rr.Code == http.StatusFound && strings.Contains(rr.Header().Get("Location"), "gmail=connected") {
		t.Error("forged state produced a success redirect")
	}
}

// TestGmail_Callback_ExpiredState_NoSave — a state past its TTL (the
// user sat on the consent screen too long) is rejected, not saved.
func TestGmail_Callback_ExpiredState_NoSave(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	state := signGmailState(t, secret, uuid.New(), uuid.New(), time.Now().Add(-time.Minute))
	svc := &fakeGmailService{fetchEmail: "x@y.com"}
	h := &GmailHandler{gmailSvc: svc, stateSecret: secret, frontendURL: "https://app.example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/callback?code=c&state="+state, nil)
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if svc.saveCalled {
		t.Error("expired state must not be saved")
	}
	if rr.Code == http.StatusFound && strings.Contains(rr.Header().Get("Location"), "gmail=connected") {
		t.Error("expired state produced a success redirect")
	}
}

// TestGmail_Callback_ExchangeError_RedirectsError — a failed code
// exchange (e.g. Google rejects the code) redirects to the error page
// and never reaches userinfo or save.
func TestGmail_Callback_ExchangeError_RedirectsError(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	state := signGmailState(t, secret, uuid.New(), uuid.New(), time.Now().Add(10*time.Minute))
	svc := &fakeGmailService{exchangeErr: errors.New("invalid_grant")}
	h := &GmailHandler{gmailSvc: svc, stateSecret: secret, frontendURL: "https://app.example.com"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/callback?code=bad&state="+state, nil)
	rr := httptest.NewRecorder()

	h.Callback(rr, req)

	if rr.Code != http.StatusFound || !strings.Contains(rr.Header().Get("Location"), "gmail=error") {
		t.Errorf("status=%d loc=%q want 302 -> gmail=error", rr.Code, rr.Header().Get("Location"))
	}
	if svc.fetchCalled || svc.saveCalled {
		t.Error("a failed exchange must not reach userinfo or save")
	}
}

// TestGmail_AuthURL_SignsTenantAndUser — the state minted for the
// connect URL must be a verifiable signed token carrying the real
// tenant + the real connecting user (resolved from the Clerk id), not
// the old opaque random string. This is what lets the public callback
// trust the binding.
func TestGmail_AuthURL_SignsTenantAndUser(t *testing.T) {
	secret := []byte("test-gmail-state-secret-0123456789")
	tenantID := uuid.New()
	userID := uuid.New()

	svc := &fakeGmailService{}
	q := &fakeGmailQueries{user: repository.User{ID: pgtype.UUID{Bytes: userID, Valid: true}}}
	h := &GmailHandler{gmailSvc: svc, queries: q, stateSecret: secret}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/auth-url", nil)
	req = withTenant(req, tenantID)
	req = withUserClerkID(req, "user_clerk_123")
	rr := httptest.NewRecorder()

	h.AuthURL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		URL   string `json:"url"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.State == "" {
		t.Fatal("auth-url returned an empty state")
	}
	claims, err := jwt.Decode(resp.State, secret)
	if err != nil {
		t.Fatalf("state is not a valid signed token: %v", err)
	}
	if claims.TenantID != tenantID.String() {
		t.Errorf("state tenant=%q want %q", claims.TenantID, tenantID.String())
	}
	if claims.UserID != userID.String() {
		t.Errorf("state user=%q want %q (must be the real user, not the tenant)", claims.UserID, userID.String())
	}
	if !strings.Contains(resp.URL, "state="+resp.State) {
		t.Errorf("auth URL %q does not embed state %q", resp.URL, resp.State)
	}
}

// TestGmail_AuthURL_UserLookupFails — if the connecting Clerk id has no
// user row, AuthURL must fail rather than mint a state with an empty
// user that would later FK-violate on save.
func TestGmail_AuthURL_UserLookupFails(t *testing.T) {
	svc := &fakeGmailService{}
	q := &fakeGmailQueries{userErr: errors.New("no rows")}
	h := &GmailHandler{gmailSvc: svc, queries: q, stateSecret: []byte("s")}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/gmail/auth-url", nil)
	req = withTenant(req, uuid.New())
	req = withUserClerkID(req, "user_clerk_unknown")
	rr := httptest.NewRecorder()

	h.AuthURL(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("status=200 but user lookup failed — should not mint a state")
	}
}
