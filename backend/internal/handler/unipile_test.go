package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// fakeUnipileService stands in for the outbound Unipile calls
// (HostedAuthLink, Disconnect) and lets the webhook tests inject the
// real *unipile.Module for genuine signature/parse coverage. Tests set
// only the fields they exercise.
type fakeUnipileService struct {
	configured      bool
	authURL         string
	authErr         error
	capturedMeta    string
	capturedParams  unipile.HostedAuthParams
	disconnectErr   error
	disconnectedID  string
	disconnectCalls int
	order           *[]string // shared call-log for ordering assertions
}

func (f *fakeUnipileService) HostedAuthLink(ctx context.Context, p unipile.HostedAuthParams) (string, error) {
	f.capturedMeta = p.Metadata
	f.capturedParams = p
	return f.authURL, f.authErr
}
func (f *fakeUnipileService) Disconnect(ctx context.Context, id string) error {
	f.disconnectCalls++
	f.disconnectedID = id
	if f.order != nil {
		*f.order = append(*f.order, "disconnect")
	}
	return f.disconnectErr
}
func (f *fakeUnipileService) Configured() bool        { return f.configured }
func (f *fakeUnipileService) WebhookConfigured() bool { return true }
func (f *fakeUnipileService) ParseWebhook(b []byte) (unipile.Event, error) {
	return unipile.New("", "", nil, nil).ParseWebhook(b)
}
func (f *fakeUnipileService) VerifyAuthToken(token string) bool { return false }

// fakeUnipileQueries captures the repository calls each handler makes.
type fakeUnipileQueries struct {
	user        repository.User
	userErr     error
	connCount   int64
	connErr     error
	account     repository.LinkedinAccount
	getErr      error
	listResult  []repository.LinkedinAccount
	deleteErr   error
	upserted    repository.UpsertLinkedInAccountParams
	upsertCalls int
	statusSet   repository.SetLinkedInAccountStatusByUnipileIDParams
	statusCalls int
	getCalls    int
	deleteCalls int
	order       *[]string // shared call-log for ordering assertions

	acceptedLeadID pgtype.UUID
	acceptedErr    error
	eventCalls     int
	lastEvent      repository.CreateLinkedInEventParams

	chatLeadID  pgtype.UUID
	chatLeadErr error
}

func (f *fakeUnipileQueries) GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error) {
	return f.user, f.userErr
}
func (f *fakeUnipileQueries) CountConnectedLinkedInAccounts(ctx context.Context, tenantID pgtype.UUID) (int64, error) {
	return f.connCount, f.connErr
}
func (f *fakeUnipileQueries) UpsertLinkedInAccount(ctx context.Context, arg repository.UpsertLinkedInAccountParams) (repository.LinkedinAccount, error) {
	f.upsertCalls++
	f.upserted = arg
	return repository.LinkedinAccount{TenantID: arg.TenantID, UnipileAccountID: arg.UnipileAccountID, Status: arg.Status}, nil
}
func (f *fakeUnipileQueries) SetLinkedInAccountStatusByUnipileID(ctx context.Context, arg repository.SetLinkedInAccountStatusByUnipileIDParams) error {
	f.statusCalls++
	f.statusSet = arg
	return nil
}
func (f *fakeUnipileQueries) GetLinkedInAccount(ctx context.Context, arg repository.GetLinkedInAccountParams) (repository.LinkedinAccount, error) {
	f.getCalls++
	return f.account, f.getErr
}
func (f *fakeUnipileQueries) ListLinkedInAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.LinkedinAccount, error) {
	return f.listResult, nil
}
func (f *fakeUnipileQueries) DeleteLinkedInAccount(ctx context.Context, arg repository.DeleteLinkedInAccountParams) error {
	f.deleteCalls++
	if f.order != nil {
		*f.order = append(*f.order, "delete")
	}
	return f.deleteErr
}

// acceptance-webhook seam (issue #5). The handler's accept path is
// exercised end-to-end against the real DB in
// unipile_accept_integration_test.go; these stubs only keep the unit
// suite compiling and aren't asserted on by the connect/disconnect tests.
func (f *fakeUnipileQueries) MarkLinkedInAccepted(ctx context.Context, invitationID string) (pgtype.UUID, error) {
	return f.acceptedLeadID, f.acceptedErr
}
func (f *fakeUnipileQueries) CreateLinkedInEvent(ctx context.Context, arg repository.CreateLinkedInEventParams) (repository.LinkedinEvent, error) {
	f.eventCalls++
	f.lastEvent = arg
	return repository.LinkedinEvent{}, nil
}

// inbound-message seam (issue #6). The reply path is exercised end-to-end
// against the real DB in unipile_reply_integration_test.go; this stub only
// keeps the unit suite compiling.
func (f *fakeUnipileQueries) GetLinkedInLeadByChatID(ctx context.Context, chatID string) (pgtype.UUID, error) {
	return f.chatLeadID, f.chatLeadErr
}

const testUnipileSecret = "test-unipile-webhook-secret-0123456789"

// readWebhookFixture loads a frozen real Unipile payload from testdata.
// These are the bytes captured live during the #1 validation spike; the
// parser and handler must accept them exactly as Unipile sends them.
func readWebhookFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestUnipile_Webhook_MessageReceived_RealPayload_200 is the end-to-end
// tracer: the real message_received bytes captured in the #1 spike — the
// exact payload that 400'd in production because is_sender arrives as a
// bool, not the int the wire struct declared — POSTed with the static
// Unipile-Auth header real Unipile uses, must authenticate, parse, route
// as an inbound message, and answer 200. reply is left nil so the message
// path no-ops (acting on the reply is #8); this slice only proves the
// auth + parse + route skeleton the reply behavior hangs on.
func TestUnipile_Webhook_MessageReceived_RealPayload_200(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil) // real crypto + parse, no outbound HTTP
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: secret}

	body := readWebhookFixture(t, "webhook_message_received_inbound.json")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	rr := httptest.NewRecorder()

	h.Webhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (real message_received must auth+parse+200) body=%s", rr.Code, rr.Body.String())
	}
}

// hmacHex reproduces the legacy body-HMAC the old X-Unipile-Signature
// path used, so a test can prove that even a correctly-signed body is now
// rejected. Kept local to the test — the production Sign method is gone.
func hmacHex(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestUnipile_Webhook_UnhandledEvent_200 pins AC-1's pass-through: a
// well-formed messaging event we don't act on (a read receipt here)
// authenticates, parses to EventUnhandled, and answers 200 with no side
// effect — so Unipile stops retrying rather than seeing a 4xx/5xx.
func TestUnipile_Webhook_UnhandledEvent_200(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: secret}

	body := `{"event":"message_read","account_id":"acct_test_0001","chat_id":"chat_x"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(body))
	req.Header.Set("Unipile-Auth", testUnipileSecret)
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 for an event we don't act on", rr.Code)
	}
	if q.upsertCalls != 0 || q.statusCalls != 0 || q.eventCalls != 0 {
		t.Error("an unhandled event must not touch the repository")
	}
}

// TestUnipile_Webhook_HMACSignatureRejected_401 locks AC-2: the legacy
// self-signed X-Unipile-Signature path is gone. A body carrying a VALID
// body-HMAC but no Unipile-Auth header must be rejected — real Unipile
// authenticates only with the static header.
func TestUnipile_Webhook_HMACSignatureRejected_401(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: secret}

	body := readWebhookFixture(t, "webhook_message_received_inbound.json")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("X-Unipile-Signature", hmacHex(secret, body)) // valid HMAC, no longer trusted
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 — a valid HMAC signature must no longer authenticate", rr.Code)
	}
}

// The account.connected → upsert path (connect-and-bind) is issue #3's
// slice. Its real notify_url payload was never captured in the #1 spike
// (finding A: hosted-auth carries no notify_url, so no tenant metadata
// echoes back), so this slice deliberately does not assert it against an
// imagined flat {status,account_id,name} shape — #3 adds the real connect
// fixture and the binding test once that payload is captured.

func TestUnipile_Webhook_NoSecret_503(t *testing.T) {
	mod := unipile.New("", "", nil, nil) // no webhook secret → not configured
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: nil}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503 when webhook secret unconfigured", rr.Code)
	}
	if q.upsertCalls != 0 {
		t.Error("must not upsert when verification is unconfigured")
	}
}

func TestUnipile_Webhook_BadAuth_401(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: secret}

	body := readWebhookFixture(t, "webhook_message_received_inbound.json")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	req.Header.Set("Unipile-Auth", "not-the-secret") // wrong token
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 on an incorrect Unipile-Auth header", rr.Code)
	}
	if q.upsertCalls != 0 || q.statusCalls != 0 {
		t.Error("must not touch the repository on auth failure")
	}
}

func TestUnipile_Webhook_MissingAuth_401(t *testing.T) {
	secret := []byte(testUnipileSecret)
	mod := unipile.New("", "", secret, nil)
	q := &fakeUnipileQueries{}
	h := &UnipileHandler{svc: mod, queries: q, stateSecret: secret}

	body := readWebhookFixture(t, "webhook_message_received_inbound.json")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/unipile", strings.NewReader(string(body)))
	// no Unipile-Auth header at all
	rr := httptest.NewRecorder()
	h.Webhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 when the Unipile-Auth header is absent", rr.Code)
	}
}

func TestUnipile_AuthURL_SignsMetadataAndReturnsURL(t *testing.T) {
	secret := []byte(testUnipileSecret)
	tenantID := uuid.New()
	userID := uuid.New()

	svc := &fakeUnipileService{configured: true, authURL: "https://account.unipile.com/wizard123"}
	q := &fakeUnipileQueries{user: repository.User{ID: pgtype.UUID{Bytes: userID, Valid: true}}, connCount: 0}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: secret, frontendURL: "https://app.test"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/auth-url", nil)
	req = withTenant(req, tenantID)
	req = withUserClerkID(req, "user_clerk_1")
	rr := httptest.NewRecorder()
	h.AuthURL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.URL != "https://account.unipile.com/wizard123" {
		t.Errorf("url=%q want the hosted wizard url", resp.URL)
	}
	// The metadata handed to Unipile must be a signed token carrying the
	// real tenant + connecting user, recoverable on the webhook.
	claims, err := jwt.Decode(svc.capturedMeta, secret)
	if err != nil {
		t.Fatalf("metadata is not a valid signed token: %v", err)
	}
	if claims.TenantID != tenantID.String() {
		t.Errorf("metadata tenant=%q want %q", claims.TenantID, tenantID.String())
	}
	if claims.UserID != userID.String() {
		t.Errorf("metadata user=%q want %q", claims.UserID, userID.String())
	}
}

// TestUnipile_AuthURL_SetsNotifyURL proves AuthURL wires the connect callback
// channel (issue #3, finding A): the hosted-auth request it builds carries a
// notify_url pointing at this backend's public webhook endpoint, so Unipile
// echoes the signed metadata back and the account can bind. Built from apiURL
// (APP_URL) + the fixed webhook path.
func TestUnipile_AuthURL_SetsNotifyURL(t *testing.T) {
	secret := []byte(testUnipileSecret)
	svc := &fakeUnipileService{configured: true, authURL: "https://account.unipile.com/wizard123"}
	q := &fakeUnipileQueries{user: repository.User{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, connCount: 0}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: secret, frontendURL: "https://app.test", apiURL: "https://api.test"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/auth-url", nil)
	req = withTenant(req, uuid.New())
	req = withUserClerkID(req, "user_clerk_1")
	rr := httptest.NewRecorder()
	h.AuthURL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	if got, want := svc.capturedParams.NotifyURL, "https://api.test/api/v1/webhooks/unipile"; got != want {
		t.Errorf("notify_url=%q want %q", got, want)
	}
}

// TestUnipile_AuthURL_NoAPIURL_OmitsNotifyURL proves the degrade path: with no
// public base URL configured, AuthURL omits notify_url rather than emitting a
// broken relative one — the account simply won't auto-bind, same as before the
// callback was wired.
func TestUnipile_AuthURL_NoAPIURL_OmitsNotifyURL(t *testing.T) {
	secret := []byte(testUnipileSecret)
	svc := &fakeUnipileService{configured: true, authURL: "https://account.unipile.com/wizard123"}
	q := &fakeUnipileQueries{user: repository.User{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, connCount: 0}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: secret, frontendURL: "https://app.test"} // apiURL empty

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/auth-url", nil)
	req = withTenant(req, uuid.New())
	req = withUserClerkID(req, "user_clerk_1")
	rr := httptest.NewRecorder()
	h.AuthURL(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	if svc.capturedParams.NotifyURL != "" {
		t.Errorf("notify_url=%q want empty when no public URL is configured", svc.capturedParams.NotifyURL)
	}
}

func TestUnipile_AuthURL_FreeTierGate_4xx(t *testing.T) {
	svc := &fakeUnipileService{configured: true, authURL: "https://account.unipile.com/should-not-be-used"}
	q := &fakeUnipileQueries{user: repository.User{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}, connCount: 1}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: []byte(testUnipileSecret)}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/auth-url", nil)
	req = withTenant(req, uuid.New())
	req = withUserClerkID(req, "user_clerk_1")
	rr := httptest.NewRecorder()
	h.AuthURL(rr, req)

	if rr.Code < 400 || rr.Code >= 500 {
		t.Errorf("status=%d want a 4xx capacity error on the second connect", rr.Code)
	}
	if svc.capturedMeta != "" {
		t.Error("HostedAuthLink must not be called once the free-tier limit is hit")
	}
}

func TestUnipile_AuthURL_NotConfigured_503(t *testing.T) {
	svc := &fakeUnipileService{configured: false}
	q := &fakeUnipileQueries{user: repository.User{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}}}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: []byte(testUnipileSecret)}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/auth-url", nil)
	req = withTenant(req, uuid.New())
	req = withUserClerkID(req, "user_clerk_1")
	rr := httptest.NewRecorder()
	h.AuthURL(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d want 503 when UNIPILE_API_KEY absent (degrade, no fatal)", rr.Code)
	}
}

func TestUnipile_DeleteAccount_DisconnectsThenDeletes(t *testing.T) {
	accountID := uuid.New()
	tenantID := uuid.New()
	order := &[]string{}
	svc := &fakeUnipileService{configured: true, order: order}
	q := &fakeUnipileQueries{
		order: order,
		account: repository.LinkedinAccount{
			ID:               pgtype.UUID{Bytes: accountID, Valid: true},
			TenantID:         pgtype.UUID{Bytes: tenantID, Valid: true},
			UnipileAccountID: "acc_to_remove",
			Status:           "active",
		},
	}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: []byte(testUnipileSecret)}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/linkedin/accounts/"+accountID.String(), nil)
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": accountID.String()})
	rr := httptest.NewRecorder()

	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if svc.disconnectedID != "acc_to_remove" {
		t.Errorf("disconnected id=%q want acc_to_remove", svc.disconnectedID)
	}
	// Disconnect at Unipile must precede the local row delete — so a
	// Unipile outage never strands a still-live session behind a deleted
	// row (mirrors the Gmail revoke-then-delete invariant).
	if strings.Join(*order, ",") != "disconnect,delete" {
		t.Errorf("call order=%v want [disconnect delete]", *order)
	}
}

func TestUnipile_DeleteAccount_NotFound_204(t *testing.T) {
	svc := &fakeUnipileService{configured: true}
	q := &fakeUnipileQueries{getErr: errors.New("no rows")}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: []byte(testUnipileSecret)}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/linkedin/accounts/"+uuid.NewString(), nil)
	req = withTenant(req, uuid.New())
	req = withURLParams(req, map[string]string{"id": uuid.NewString()})
	rr := httptest.NewRecorder()
	h.DeleteAccount(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204 (idempotent delete)", rr.Code)
	}
	if svc.disconnectCalls != 0 || q.deleteCalls != 0 {
		t.Error("must not disconnect or delete when the account doesn't exist")
	}
}

// TestMapUnipileStatus pins the account-status vocabulary → enum mapping
// (issue #4). Only CREATION_SUCCESS is a spike-confirmed real value — the
// sole status the #1 validation spike ever captured (see
// docs/2026-06-25-finish-unipile-integration/spike-captures.md §1). The
// restriction/disconnect strings are defensive best-effort mappings no
// capture could confirm (finding E: those transitions aren't emittable on
// demand). An unrecognised value maps to "" so the caller leaves the stored
// status untouched rather than guessing (the no-clobber contract).
func TestMapUnipileStatus(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"CREATION_SUCCESS", "active"}, // the one spike-confirmed real value
		{"OK", "active"},
		{"OK_PENDING", "active"},
		{"CONNECTED", "active"},
		{"SYNC_SUCCESS", "active"},
		{"ERROR", "restricted"},
		{"CREDENTIALS", "restricted"},
		{"STOPPED", "restricted"},
		{"PERMISSIONS", "restricted"},
		{"DELETED", "disconnected"},
		{"DISCONNECTED", "disconnected"},
		{"creation_success", "active"},     // case-insensitive
		{"  CREATION_SUCCESS  ", "active"}, // surrounding whitespace trimmed
		{"SOMETHING_NEW", ""},              // unknown provider value → no map
		{"", ""},                           // empty → no map
	}
	for _, tc := range cases {
		if got := mapUnipileStatus(tc.raw); got != tc.want {
			t.Errorf("mapUnipileStatus(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestUnipile_ListAccounts(t *testing.T) {
	tenantID := uuid.New()
	svc := &fakeUnipileService{configured: true}
	q := &fakeUnipileQueries{listResult: []repository.LinkedinAccount{
		{UnipileAccountID: "acc_1", Status: "active"},
	}}
	h := &UnipileHandler{svc: svc, queries: q, stateSecret: []byte(testUnipileSecret)}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/linkedin/accounts", nil)
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()
	h.ListAccounts(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rr.Code)
	}
	var got []repository.LinkedinAccount
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Status != "active" {
		t.Errorf("accounts=%+v want one active row", got)
	}
}
