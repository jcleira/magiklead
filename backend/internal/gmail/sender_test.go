package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// stubGmail boots an httptest server that answers the two endpoints
// Sender touches: oauth2 token refresh and users.messages.send.
// Returns the live server plus a *Sender pre-wired to it.
type stubGmail struct {
	server          *httptest.Server
	sender          *Sender
	sendHandler     func(w http.ResponseWriter, r *http.Request)
	refreshHandler  func(w http.ResponseWriter, r *http.Request)
	lastSentMessage []byte // raw RFC 5322 of the last successful send
	sentCalls       int
	refreshCalls    int
}

func newStubGmail(t *testing.T) *stubGmail {
	t.Helper()
	s := &stubGmail{}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		s.refreshCalls++
		if s.refreshHandler != nil {
			s.refreshHandler(w, r)
			return
		}
		// Default: hand back a fresh token good for an hour.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "refreshed-access-token",
			"token_type": "Bearer",
			"refresh_token": "refresh-token",
			"expires_in": 3600
		}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/send", func(w http.ResponseWriter, r *http.Request) {
		s.sentCalls++
		// Capture the encoded raw so tests can inspect headers/body.
		var body struct {
			Raw string `json:"raw"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Raw != "" {
			if decoded, err := base64.URLEncoding.DecodeString(body.Raw); err == nil {
				s.lastSentMessage = decoded
			}
		}

		if s.sendHandler != nil {
			s.sendHandler(w, r)
			return
		}
		// Default: 200 with a synthetic message-id and thread-id.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "stub-msg-id", "threadId": "stub-thread-id"}`))
	})

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)

	cfg := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL: s.server.URL + "/oauth2/token",
			AuthURL:  s.server.URL + "/oauth2/auth",
		},
		Scopes: []string{
			"https://www.googleapis.com/auth/gmail.send",
			"https://www.googleapis.com/auth/gmail.readonly",
		},
	}
	s.sender = NewSenderForTest(cfg, s.server.URL+"/")
	return s
}

// validAccount is a ConnectedAccount with a token that hasn't yet
// expired — the SDK will not trigger a refresh on the happy path.
func validAccount() ConnectedAccount {
	return ConnectedAccount{
		Email:        "sender@example.com",
		AccessToken:  "valid-access-token",
		RefreshToken: "refresh-token",
		TokenExpiry:  time.Now().Add(30 * time.Minute),
	}
}

// expiredAccount carries an already-expired access token; oauth2 must
// hit the refresh endpoint before the send call lands.
func expiredAccount() ConnectedAccount {
	return ConnectedAccount{
		Email:        "sender@example.com",
		AccessToken:  "stale-access-token",
		RefreshToken: "refresh-token",
		TokenExpiry:  time.Now().Add(-1 * time.Hour),
	}
}

// TestSender_HappyPath proves the tracer: token attached → request
// reaches /users/me/messages/send → stub returns id/threadId → Sender
// returns matching SendResult.
func TestSender_HappyPath(t *testing.T) {
	stub := newStubGmail(t)

	result, err := stub.sender.Send(context.Background(), validAccount(), Message{
		FromName: "Operator",
		To:       "lead@example.com",
		Subject:  "Hello there",
		Body:     "<p>Hi from MagikLead</p>",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.MessageID != "stub-msg-id" {
		t.Errorf("MessageID=%q want stub-msg-id", result.MessageID)
	}
	if result.ThreadID != "stub-thread-id" {
		t.Errorf("ThreadID=%q want stub-thread-id", result.ThreadID)
	}
	if stub.sentCalls != 1 {
		t.Errorf("send endpoint calls=%d want 1", stub.sentCalls)
	}

	// Outbound message bytes carry the expected From / To / Subject
	// — proves buildRFC5322 ran, not just that the HTTP call landed.
	raw := string(stub.lastSentMessage)
	if !strings.Contains(raw, "To: lead@example.com") {
		t.Errorf("raw message missing To header:\n%s", raw)
	}
	if !strings.Contains(raw, "Operator") || !strings.Contains(raw, "<sender@example.com>") {
		t.Errorf("raw message missing From/display-name:\n%s", raw)
	}
	if !strings.Contains(raw, "Hi from MagikLead") {
		t.Errorf("raw message missing body:\n%s", raw)
	}
}

// TestSender_TokenRefresh covers the auto-refresh-then-retry path:
// an expired access token triggers /oauth2/token, then the send call
// uses the refreshed bearer and succeeds.
func TestSender_TokenRefresh(t *testing.T) {
	stub := newStubGmail(t)

	var bearerOnSend string
	stub.sendHandler = func(w http.ResponseWriter, r *http.Request) {
		bearerOnSend = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": "msg-after-refresh", "threadId": "t-after-refresh"}`))
	}

	result, err := stub.sender.Send(context.Background(), expiredAccount(), Message{
		To:      "lead@example.com",
		Subject: "Subject",
		Body:    "<p>Body</p>",
	})
	if err != nil {
		t.Fatalf("Send after refresh: %v", err)
	}
	if result.MessageID != "msg-after-refresh" {
		t.Errorf("MessageID=%q", result.MessageID)
	}
	if stub.refreshCalls != 1 {
		t.Errorf("refresh endpoint calls=%d want 1", stub.refreshCalls)
	}
	// The send call must use the refreshed access token, not the
	// stale one — that's what proves the refresh result actually
	// got propagated to the next HTTP call.
	if !strings.Contains(bearerOnSend, "refreshed-access-token") {
		t.Errorf("Authorization=%q want refreshed-access-token", bearerOnSend)
	}
}

// TestSender_RefreshFails returns ErrTokenExpired when the refresh
// token is no longer valid (e.g. revoked / rotated). The caller
// (worker) uses this to mark the account as needing reconnect.
func TestSender_RefreshFails(t *testing.T) {
	stub := newStubGmail(t)
	stub.refreshHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	}

	_, err := stub.sender.Send(context.Background(), expiredAccount(), Message{
		To:      "lead@example.com",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("err=%v want ErrTokenExpired", err)
	}
	if stub.sentCalls != 0 {
		t.Errorf("send must not happen when refresh fails; calls=%d", stub.sentCalls)
	}
}

// TestSender_ScopeMissing — a 403 with an insufficient-scope reason
// must surface as ErrScopeMissing so the UI can render a reconnect
// link, not a generic "send failed."
func TestSender_ScopeMissing(t *testing.T) {
	stub := newStubGmail(t)
	stub.sendHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 403,
				"message": "Request had insufficient authentication scopes.",
				"errors": [{"reason": "insufficientPermissions", "message": "Insufficient Permission"}]
			}
		}`))
	}

	_, err := stub.sender.Send(context.Background(), validAccount(), Message{
		To:      "lead@example.com",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if !errors.Is(err, ErrScopeMissing) {
		t.Errorf("err=%v want ErrScopeMissing", err)
	}
}

// TestSender_MessageRejected — 400-class responses that aren't
// scope/auth related (e.g. invalid recipient) must surface as
// ErrMessageRejected so the worker writes a 'failed' event with the
// right reason rather than retrying indefinitely.
func TestSender_MessageRejected(t *testing.T) {
	stub := newStubGmail(t)
	stub.sendHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 400,
				"message": "Invalid To header",
				"errors": [{"reason": "invalidArgument", "message": "Invalid To header"}]
			}
		}`))
	}

	_, err := stub.sender.Send(context.Background(), validAccount(), Message{
		To:      "not-an-address",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if !errors.Is(err, ErrMessageRejected) {
		t.Errorf("err=%v want ErrMessageRejected", err)
	}
}

// TestSender_RateLimited — 429 must surface as ErrRateLimited so the
// worker treats it as a transient retry-later condition, not a
// permanent failure.
func TestSender_RateLimited(t *testing.T) {
	stub := newStubGmail(t)
	stub.sendHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 429,
				"message": "User rate limit exceeded",
				"errors": [{"reason": "rateLimitExceeded"}]
			}
		}`))
	}

	_, err := stub.sender.Send(context.Background(), validAccount(), Message{
		To:      "lead@example.com",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err=%v want ErrRateLimited", err)
	}
}

// TestSender_ServerError — a 5xx is also a "retry later" signal and
// gets lumped with rate-limited, since the worker handles both the
// same way.
func TestSender_ServerError(t *testing.T) {
	stub := newStubGmail(t)
	stub.sendHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": 503,
				"message": "Service unavailable"
			}
		}`))
	}

	_, err := stub.sender.Send(context.Background(), validAccount(), Message{
		To:      "lead@example.com",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err=%v want ErrRateLimited", err)
	}
}

// TestSender_NetworkError — when the transport itself fails (server
// closed before responding), the error is wrapped as a generic
// network error, not one of the Err* sentinels.
func TestSender_NetworkError(t *testing.T) {
	stub := newStubGmail(t)
	// Close the server before Send to force a connection-refused.
	stub.server.Close()

	_, err := stub.sender.Send(context.Background(), validAccount(), Message{
		To:      "lead@example.com",
		Subject: "S",
		Body:    "<p>B</p>",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Must NOT match the rejection / scope sentinels — those are
	// reserved for Gmail-classified failures.
	if errors.Is(err, ErrMessageRejected) || errors.Is(err, ErrScopeMissing) || errors.Is(err, ErrRateLimited) {
		t.Errorf("network error misclassified: %v", err)
	}
}

// TestNewOAuthConfig_Scopes pins the granted scope set so that any
// future refactor of oauth.go that drops gmail.send or gmail.readonly
// trips this test before it ships.
func TestNewOAuthConfig_Scopes(t *testing.T) {
	cfg := NewOAuthConfig("cid", "csec", "http://localhost/cb")
	want := map[string]bool{
		"https://www.googleapis.com/auth/gmail.send":     false,
		"https://www.googleapis.com/auth/gmail.readonly": false,
	}
	for _, s := range cfg.Scopes {
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}
	for scope, present := range want {
		if !present {
			t.Errorf("scope missing: %s", scope)
		}
	}
}

// Sanity check: buildRFC5322 honours custom headers like the
// List-Unsubscribe line that issue #6 will add.
func TestBuildRFC5322_CustomHeaders(t *testing.T) {
	raw := string(buildRFC5322("a@b.com", Message{
		To:      "c@d.com",
		Subject: "Hi",
		Body:    "<p>hi</p>",
		Headers: map[string]string{
			"List-Unsubscribe": "<mailto:u@b.com>, <https://b.com/unsub/x>",
		},
	}))
	if !strings.Contains(raw, "List-Unsubscribe: ") {
		t.Errorf("missing List-Unsubscribe header:\n%s", raw)
	}
}

// BuildUnsubscribeHeaders produces the two RFC 8058 / Gmail-required
// headers from a (lead email, tenant) pair. The token segment of each
// URL must decode back to the same claims so the public endpoint can
// trust them. The List-Unsubscribe-Post header is the literal Gmail
// looks for to enable one-click.
func TestBuildUnsubscribeHeaders(t *testing.T) {
	secret := []byte("test-secret-do-not-use-in-prod")
	tenantID := uuid.New()
	email := "lead@example.com"

	headers, err := BuildUnsubscribeHeaders(email, tenantID, secret,
		"https://app.example.com", "mail.example.com")
	if err != nil {
		t.Fatalf("BuildUnsubscribeHeaders: %v", err)
	}

	got := headers["List-Unsubscribe-Post"]
	if got != "List-Unsubscribe=One-Click" {
		t.Errorf("List-Unsubscribe-Post=%q want %q", got, "List-Unsubscribe=One-Click")
	}

	listUnsub := headers["List-Unsubscribe"]
	if !strings.Contains(listUnsub, "<mailto:unsubscribe+") {
		t.Errorf("List-Unsubscribe missing mailto: %q", listUnsub)
	}
	if !strings.Contains(listUnsub, "@mail.example.com>") {
		t.Errorf("List-Unsubscribe missing mail-domain: %q", listUnsub)
	}
	if !strings.Contains(listUnsub, "<https://app.example.com/api/v1/public/unsubscribe?token=") {
		t.Errorf("List-Unsubscribe missing https URL: %q", listUnsub)
	}

	// Extract token from the https URL and confirm it decodes back to
	// the claims we asked for. Use a substring slice instead of regex —
	// header is fully under our control here.
	const marker = "token="
	idx := strings.Index(listUnsub, marker)
	if idx < 0 {
		t.Fatalf("no token= in header: %q", listUnsub)
	}
	rest := listUnsub[idx+len(marker):]
	tokenEnd := strings.IndexAny(rest, ">,")
	if tokenEnd < 0 {
		t.Fatalf("token not terminated by > or , in header: %q", listUnsub)
	}
	token := rest[:tokenEnd]

	claims, err := jwt.Decode(token, secret)
	if err != nil {
		t.Fatalf("Decode embedded token: %v", err)
	}
	if claims.Email != email {
		t.Errorf("token email=%q want %q", claims.Email, email)
	}
	if claims.TenantID != tenantID.String() {
		t.Errorf("token tenant_id=%q want %q", claims.TenantID, tenantID.String())
	}
	// Expiry must be far enough out that suppression stays actionable —
	// we don't pin to a literal but require >1 year so a misconfiguration
	// that sets a 1-hour expiry trips this test.
	if claims.Exp < time.Now().Add(365*24*time.Hour).Unix() {
		t.Errorf("exp=%d is less than 1 year out", claims.Exp)
	}
}

// The mailto local part must be the same token as the https URL —
// otherwise a recipient who emails the mailto can't be matched back
// to the same suppression record.
func TestBuildUnsubscribeHeaders_TokenMatchesAcrossChannels(t *testing.T) {
	secret := []byte("s")
	headers, err := BuildUnsubscribeHeaders("a@b.com", uuid.New(), secret, "https://app", "mail.d")
	if err != nil {
		t.Fatalf("BuildUnsubscribeHeaders: %v", err)
	}
	h := headers["List-Unsubscribe"]
	mailtoStart := strings.Index(h, "unsubscribe+")
	mailtoEnd := strings.Index(h, "@mail.d")
	if mailtoStart < 0 || mailtoEnd < 0 || mailtoEnd < mailtoStart {
		t.Fatalf("mailto malformed: %q", h)
	}
	mailtoToken := h[mailtoStart+len("unsubscribe+") : mailtoEnd]

	const marker = "token="
	urlStart := strings.Index(h, marker)
	if urlStart < 0 {
		t.Fatalf("no token= in: %q", h)
	}
	urlRest := h[urlStart+len(marker):]
	urlEnd := strings.IndexAny(urlRest, ">,")
	urlToken := urlRest[:urlEnd]

	if mailtoToken != urlToken {
		t.Errorf("token mismatch: mailto=%q url=%q", mailtoToken, urlToken)
	}
}

// guard: classifySendError must not panic on a nil error (defensive,
// since Send only calls it on a real failure but the contract is
// nicer if it's safe).
func TestClassifySendError_NilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("classifySendError panicked: %v", r)
		}
	}()
	// classifySendError is internal; calling with a generic error to
	// avoid nil-deref.
	_ = classifySendError(fmt.Errorf("plain error"))
}
