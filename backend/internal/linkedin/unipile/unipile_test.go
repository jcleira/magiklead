package unipile_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// stub wraps an httptest.Server with capture so tests can assert the
// outbound request shape — the LinkedIn analogue of the PDL Doer stub.
type stub struct {
	*httptest.Server
	method   string
	path     string
	rawQuery string
	lastBody []byte
}

func newStub(t *testing.T, status int, body string) *stub {
	t.Helper()
	s := &stub{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.method = r.Method
		s.path = r.URL.Path
		s.rawQuery = r.URL.RawQuery
		s.lastBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestConfigured(t *testing.T) {
	if unipile.New("", "https://dsn.test", nil, nil).Configured() {
		t.Error("empty api key must report not configured")
	}
	if !unipile.New("key", "https://dsn.test", nil, nil).Configured() {
		t.Error("api key set must report configured")
	}
	if unipile.New("key", "https://dsn.test", nil, nil).WebhookConfigured() {
		t.Error("nil webhook secret must report webhook not configured")
	}
	if !unipile.New("key", "https://dsn.test", []byte("s"), nil).WebhookConfigured() {
		t.Error("webhook secret set must report webhook configured")
	}
}

func TestHostedAuthLink_EmbedsMetadataAndReturnsURL(t *testing.T) {
	s := newStub(t, 200, `{"object":"HostedAuthUrl","url":"https://account.unipile.com/abc123"}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	url, err := m.HostedAuthLink(context.Background(), unipile.HostedAuthParams{
		Metadata:           "signed-metadata-token",
		SuccessRedirectURL: "https://app.test/settings?linkedin=connected",
		FailureRedirectURL: "https://app.test/settings?linkedin=error",
		ExpiresOn:          time.Unix(1893456000, 0), // fixed; Date.now() banned in scripts but fine here
	})
	if err != nil {
		t.Fatalf("HostedAuthLink: %v", err)
	}
	if url != "https://account.unipile.com/abc123" {
		t.Errorf("url=%q want the hosted wizard url", url)
	}
	if s.method != http.MethodPost {
		t.Errorf("method=%q want POST", s.method)
	}
	if s.path != "/api/v1/hosted/accounts/link" {
		t.Errorf("path=%q want /api/v1/hosted/accounts/link", s.path)
	}

	var req struct {
		Type      string   `json:"type"`
		Providers []string `json:"providers"`
		APIURL    string   `json:"api_url"`
		Name      string   `json:"name"`
	}
	if err := json.Unmarshal(s.lastBody, &req); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	// The signed metadata token must ride in `name` — Unipile echoes it
	// back verbatim on the account.connected webhook, and that is the
	// only thing binding the connected account to our tenant.
	if req.Name != "signed-metadata-token" {
		t.Errorf("request name=%q want the signed metadata token", req.Name)
	}
	if req.Type != "create" {
		t.Errorf("request type=%q want create", req.Type)
	}
	if len(req.Providers) != 1 || req.Providers[0] != "LINKEDIN" {
		t.Errorf("providers=%v want [LINKEDIN]", req.Providers)
	}
}

// TestHostedAuthLink_SetsNotifyURL proves the connect-and-bind delivery
// contract (issue #3, finding A): the hosted-auth request must carry a
// notify_url so Unipile has a channel to POST the account.connected callback
// (echoing our signed `name` token) back to. Without it the callback never
// arrives and no account ever binds.
func TestHostedAuthLink_SetsNotifyURL(t *testing.T) {
	s := newStub(t, 200, `{"object":"HostedAuthUrl","url":"https://account.unipile.com/abc123"}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	const notify = "https://api.test/api/v1/webhooks/unipile"
	if _, err := m.HostedAuthLink(context.Background(), unipile.HostedAuthParams{
		Metadata:  "signed-metadata-token",
		NotifyURL: notify,
		ExpiresOn: time.Unix(1893456000, 0),
	}); err != nil {
		t.Fatalf("HostedAuthLink: %v", err)
	}

	var req struct {
		NotifyURL string `json:"notify_url"`
	}
	if err := json.Unmarshal(s.lastBody, &req); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if req.NotifyURL != notify {
		t.Errorf("notify_url=%q want %q (Unipile needs the callback channel)", req.NotifyURL, notify)
	}
}

func TestHostedAuthLink_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.HostedAuthLink(context.Background(), unipile.HostedAuthParams{}); err != unipile.ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}

func TestDisconnect(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		s := newStub(t, 200, `{}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		m.SetBaseURL(s.URL)
		if err := m.Disconnect(context.Background(), "acc_123"); err != nil {
			t.Fatalf("Disconnect: %v", err)
		}
		if s.method != http.MethodDelete {
			t.Errorf("method=%q want DELETE", s.method)
		}
		if s.path != "/api/v1/accounts/acc_123" {
			t.Errorf("path=%q want /api/v1/accounts/acc_123", s.path)
		}
	})
	t.Run("unauthorized", func(t *testing.T) {
		s := newStub(t, http.StatusUnauthorized, `{}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		m.SetBaseURL(s.URL)
		if err := m.Disconnect(context.Background(), "acc_123"); err != unipile.ErrUnauthorized {
			t.Fatalf("err=%v want ErrUnauthorized", err)
		}
	})
}

// TestCancelInvitation pins the stale-invite withdrawal call (issue #7): a
// DELETE on the sent-invitation resource, the invitation id in the path and
// the account scope in the query, with the same classified-error mapping as
// the other calls and the not-configured short-circuit.
func TestCancelInvitation(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		s := newStub(t, 200, `{}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		m.SetBaseURL(s.URL)
		if err := m.CancelInvitation(context.Background(), "acc_1", "inv_stale"); err != nil {
			t.Fatalf("CancelInvitation: %v", err)
		}
		if s.method != http.MethodDelete {
			t.Errorf("method=%q want DELETE", s.method)
		}
		// The invitation id is the path tail; the account scope rides as a
		// query param — both must reach Unipile so it cancels the right
		// invite from the right account.
		if s.path != "/api/v1/users/invite/sent/inv_stale" {
			t.Errorf("path=%q want /api/v1/users/invite/sent/inv_stale", s.path)
		}
		if !strings.Contains(s.rawQuery, "account_id=acc_1") {
			t.Errorf("query=%q want account_id=acc_1", s.rawQuery)
		}
	})
	t.Run("unauthorized", func(t *testing.T) {
		s := newStub(t, http.StatusUnauthorized, `{}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		m.SetBaseURL(s.URL)
		if err := m.CancelInvitation(context.Background(), "acc_1", "inv_stale"); err != unipile.ErrUnauthorized {
			t.Fatalf("err=%v want ErrUnauthorized", err)
		}
	})
	t.Run("not_configured", func(t *testing.T) {
		m := unipile.New("", "https://dsn.test", nil, nil)
		if err := m.CancelInvitation(context.Background(), "acc_1", "inv_stale"); err != unipile.ErrNotConfigured {
			t.Fatalf("err=%v want ErrNotConfigured", err)
		}
	})
}

// readFixture loads a frozen real Unipile webhook payload captured live in
// the #1 validation spike (redacted to synthetic ids; the shape and types
// are the real bytes). Routing these exact shapes is the whole point of
// this slice, so the tests feed the raw bytes — not hand-authored guesses.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// TestParseWebhook drives the parser off the frozen real payloads. Only
// the two shapes the #1 spike actually captured are asserted here —
// message_received (spike §9, the flat body with a top-level "event"
// marker and a BOOL is_sender) and account_status (spike §1, nested under
// "AccountStatus"). The connect (notify_url) and invitation-accepted
// webhooks were never captured (#1 findings A + the new_relation opt-out),
// so their branches are exercised by #3/#7 once those slices freeze real
// bytes — this slice does not fabricate them.
func TestParseWebhook(t *testing.T) {
	m := unipile.New("key", "", nil, nil)

	t.Run("message_received routes on the event marker (inbound prospect reply)", func(t *testing.T) {
		ev, err := m.ParseWebhook(readFixture(t, "webhook_message_received_inbound.json"))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if ev.Type != unipile.EventMessageReceived {
			t.Errorf("type=%q want %q", ev.Type, unipile.EventMessageReceived)
		}
		if ev.AccountID != "acct_test_0001" {
			t.Errorf("account=%q want acct_test_0001", ev.AccountID)
		}
		if ev.ChatID != "chat_inbound_0001" {
			t.Errorf("chat=%q want chat_inbound_0001", ev.ChatID)
		}
		if ev.MessageID != "msg_inbound_0001" {
			t.Errorf("message=%q want msg_inbound_0001", ev.MessageID)
		}
		if ev.FromSelf {
			t.Error("is_sender=false must classify as a prospect reply (FromSelf=false)")
		}
	})

	t.Run("message_received recovers the outbound direction (our own echo)", func(t *testing.T) {
		ev, err := m.ParseWebhook(readFixture(t, "webhook_message_received_outbound.json"))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if ev.Type != unipile.EventMessageReceived {
			t.Errorf("type=%q want %q", ev.Type, unipile.EventMessageReceived)
		}
		if ev.ChatID != "chat_outbound_0002" || ev.MessageID != "msg_outbound_0002" {
			t.Errorf("chat/message=%q/%q want chat_outbound_0002/msg_outbound_0002", ev.ChatID, ev.MessageID)
		}
		if !ev.FromSelf {
			t.Error("is_sender=true must classify as our own outbound echo (FromSelf=true)")
		}
	})

	t.Run("account_status routes on the AccountStatus block", func(t *testing.T) {
		ev, err := m.ParseWebhook(readFixture(t, "webhook_account_status_creation_success.json"))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if ev.Type != unipile.EventAccountStatus {
			t.Errorf("type=%q want %q", ev.Type, unipile.EventAccountStatus)
		}
		if ev.AccountID != "acct_test_0001" {
			t.Errorf("account=%q want acct_test_0001", ev.AccountID)
		}
		if ev.Status != "CREATION_SUCCESS" {
			t.Errorf("status=%q want CREATION_SUCCESS", ev.Status)
		}
	})

	t.Run("malformed json is rejected", func(t *testing.T) {
		if _, err := m.ParseWebhook([]byte(`{not json`)); err == nil {
			t.Error("want an error on a malformed body")
		}
	})
}

// TestParseWebhook_DirectionFromMemberIDs pins issue #8's core fix: message
// direction is derived from the real member-id cross-check (spike finding D)
// — the sender's member id (sender.attendee_provider_id) vs the connected
// account's own member id (account_info.user_id) — NOT the presumed is_sender
// bool. The two signals agree on every captured message, so these cases
// deliberately set them to *disagree* to prove which one wins. The final case
// pins the fallback: with no member ids on the payload, is_sender is all we
// have left.
func TestParseWebhook_DirectionFromMemberIDs(t *testing.T) {
	m := unipile.New("key", "", nil, nil)

	msg := func(sender, own string, isSender bool) []byte {
		return fmt.Appendf(nil,
			`{"event":"message_received","account_id":"acct_x","chat_id":"c1","message_id":"m1",`+
				`"is_sender":%t,"account_info":{"user_id":%q},"sender":{"attendee_provider_id":%q}}`,
			isSender, own, sender)
	}

	t.Run("own echo wins over is_sender=false (self-halt bug fixed)", func(t *testing.T) {
		// sender == account: our own DM echoed back. is_sender says inbound —
		// the member-id cross-check must override it, or we self-halt.
		ev, err := m.ParseWebhook(msg("ACoAA-own", "ACoAA-own", false))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if !ev.FromSelf {
			t.Error("sender==account_info.user_id must be FromSelf=true regardless of is_sender=false")
		}
	})

	t.Run("prospect reply wins over is_sender=true", func(t *testing.T) {
		// sender != account: a real prospect reply. is_sender says self — the
		// member-id cross-check must override it, or we miss the reply.
		ev, err := m.ParseWebhook(msg("ACoAA-prospect", "ACoAA-own", true))
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if ev.FromSelf {
			t.Error("sender!=account_info.user_id must be FromSelf=false regardless of is_sender=true")
		}
	})

	t.Run("falls back to is_sender when member ids are absent", func(t *testing.T) {
		body := []byte(`{"event":"message_received","account_id":"acct_x","chat_id":"c1","message_id":"m1","is_sender":true}`)
		ev, err := m.ParseWebhook(body)
		if err != nil {
			t.Fatalf("ParseWebhook: %v", err)
		}
		if !ev.FromSelf {
			t.Error("absent member ids must fall back to is_sender (true => FromSelf=true)")
		}
	})
}

func TestSendInvitation_HappyPath(t *testing.T) {
	s := newStub(t, 200, `{"object":"InvitationSent","invitation_id":"inv_abc"}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	res, err := m.SendInvitation(context.Background(), unipile.InviteParams{
		AccountID: "acc_1",
		Recipient: "https://www.linkedin.com/in/ada",
		Note:      "Hi Ada, loved your work on the Engine.",
	})
	if err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}
	if res.InvitationID != "inv_abc" {
		t.Errorf("invitation id=%q want inv_abc", res.InvitationID)
	}
	if s.method != http.MethodPost {
		t.Errorf("method=%q want POST", s.method)
	}
	if s.path != "/api/v1/users/invite" {
		t.Errorf("path=%q want /api/v1/users/invite", s.path)
	}

	// The worker asserts the same three fields against its own stub; here
	// we pin the wire names Unipile expects.
	var req struct {
		AccountID  string `json:"account_id"`
		ProviderID string `json:"provider_id"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(s.lastBody, &req); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if req.AccountID != "acc_1" {
		t.Errorf("account_id=%q want acc_1", req.AccountID)
	}
	if req.ProviderID != "https://www.linkedin.com/in/ada" {
		t.Errorf("provider_id=%q want the recipient identifier", req.ProviderID)
	}
	if req.Message != "Hi Ada, loved your work on the Engine." {
		t.Errorf("message=%q want the rendered note", req.Message)
	}
}

func TestSendInvitation_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.SendInvitation(context.Background(), unipile.InviteParams{}); err != unipile.ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}

func TestSendInvitation_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"unauthorized", http.StatusUnauthorized, `{}`, unipile.ErrUnauthorized},
		{"rate_limited", http.StatusTooManyRequests, `{}`, unipile.ErrRateLimited},
		{
			// LinkedIn account restriction / checkpoint surfaces as a 4xx
			// whose body names the restriction — the worker maps this to a
			// 'restricted' failed event (issue #8 escalates it later).
			name:    "restricted",
			status:  http.StatusForbidden,
			body:    `{"type":"errors/account_restricted","title":"Account is restricted"}`,
			wantErr: unipile.ErrAccountRestricted,
		},
		{"upstream", http.StatusInternalServerError, `boom`, unipile.ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.status, tc.body)
			m := unipile.New("key", s.URL, nil, s.Client())
			m.SetBaseURL(s.URL)
			_, err := m.SendInvitation(context.Background(), unipile.InviteParams{AccountID: "acc_1", Recipient: "r"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSendMessage_StartsChat(t *testing.T) {
	s := newStub(t, 200, `{"object":"ChatStarted","chat_id":"chat_99","message_id":"msg_99"}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	res, err := m.SendMessage(context.Background(), unipile.MessageParams{
		AccountID: "acc_1",
		Recipient: "https://www.linkedin.com/in/grace",
		Text:      "Thanks for connecting, Grace!",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if res.MessageID != "msg_99" {
		t.Errorf("message id=%q want msg_99", res.MessageID)
	}
	// The chat id of the freshly started conversation rides back so the
	// worker can record it and thread later steps into the same chat.
	if res.ChatID != "chat_99" {
		t.Errorf("chat id=%q want chat_99", res.ChatID)
	}
	if s.method != http.MethodPost {
		t.Errorf("method=%q want POST", s.method)
	}
	if s.path != "/api/v1/chats" {
		t.Errorf("path=%q want /api/v1/chats", s.path)
	}

	// The first DM starts a chat with the prospect as the sole attendee,
	// carrying the rendered body — these are the wire names Unipile expects.
	var req struct {
		AccountID    string   `json:"account_id"`
		AttendeesIDs []string `json:"attendees_ids"`
		Text         string   `json:"text"`
	}
	if err := json.Unmarshal(s.lastBody, &req); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if req.AccountID != "acc_1" {
		t.Errorf("account_id=%q want acc_1", req.AccountID)
	}
	if len(req.AttendeesIDs) != 1 || req.AttendeesIDs[0] != "https://www.linkedin.com/in/grace" {
		t.Errorf("attendees_ids=%v want [the recipient identifier]", req.AttendeesIDs)
	}
	if req.Text != "Thanks for connecting, Grace!" {
		t.Errorf("text=%q want the rendered DM body", req.Text)
	}
}

func TestSendMessage_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.SendMessage(context.Background(), unipile.MessageParams{}); err != unipile.ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}

func TestSendMessage_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"unauthorized", http.StatusUnauthorized, `{}`, unipile.ErrUnauthorized},
		{"rate_limited", http.StatusTooManyRequests, `{}`, unipile.ErrRateLimited},
		{
			name:    "restricted",
			status:  http.StatusForbidden,
			body:    `{"type":"errors/account_restricted","title":"Account is restricted"}`,
			wantErr: unipile.ErrAccountRestricted,
		},
		{"upstream", http.StatusInternalServerError, `boom`, unipile.ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.status, tc.body)
			m := unipile.New("key", s.URL, nil, s.Client())
			m.SetBaseURL(s.URL)
			_, err := m.SendMessage(context.Background(), unipile.MessageParams{AccountID: "acc_1", Recipient: "r", Text: "hi"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
		})
	}
}

// TestAccountActivity pins the reconcile reply listing's assembly against the
// REAL GET /api/v1/chats envelope (spike §4): the chat list carries no
// per-chat direction field and no last_message object — direction is
// per-message only. Its one inbound signal is unread_count: a chat accrues
// unread count only from the other party's messages, never our own sends, so
// unread_count>0 is an inbound reply the webhook may have missed. Already-read
// chats are filtered. Acceptance is a separate listing, AccountConnections
// (see TestAccountConnections).
func TestAccountActivity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/chats"):
			_, _ = w.Write([]byte(`{"object":"ChatList","items":[
				{"object":"Chat","id":"chat_reply","unread_count":2,"attendee_provider_id":"ACoAA-prospect","provider_id":"2-abc"},
				{"object":"Chat","id":"chat_read","unread_count":0,"attendee_provider_id":"ACoAA-other","provider_id":"2-def"}
			],"cursor":null}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	m := unipile.New("key", srv.URL, nil, srv.Client())
	m.SetBaseURL(srv.URL)

	act, err := m.AccountActivity(context.Background(), "acc_1")
	if err != nil {
		t.Fatalf("AccountActivity: %v", err)
	}
	if len(act.Replies) != 1 || act.Replies[0].ChatID != "chat_reply" {
		t.Errorf("replies=%v want one {chat_reply} (unread chats only)", act.Replies)
	}
}

func TestAccountActivity_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.AccountActivity(context.Background(), "acc_1"); err != unipile.ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}

// TestAccountConnections pins the accept check's input listing (issue #7): the
// member ids parsed out of the real GET /users/relations envelope shape frozen
// in the validation spike (spike-captures §2), and that the account id is sent.
func TestAccountConnections(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		// The real UserRelationsList envelope shape (spike-captures §2).
		_, _ = w.Write([]byte(`{
			"object": "UserRelationsList",
			"items": [
				{"object":"UserRelation","connection_urn":"urn:li:fsd_connection:1","member_id":"ACoAA1","member_urn":"urn:li:fsd_profile:ACoAA1","public_identifier":"ada","public_profile_url":"https://www.linkedin.com/in/ada/"},
				{"object":"UserRelation","connection_urn":"urn:li:fsd_connection:2","member_id":"ACoAA2","member_urn":"urn:li:fsd_profile:ACoAA2","public_identifier":"grace","public_profile_url":"https://www.linkedin.com/in/grace/"}
			],
			"cursor": null
		}`))
	}))
	t.Cleanup(srv.Close)

	m := unipile.New("key", srv.URL, nil, srv.Client())
	m.SetBaseURL(srv.URL)

	ids, err := m.AccountConnections(context.Background(), "acc_1")
	if err != nil {
		t.Fatalf("AccountConnections: %v", err)
	}
	if !slices.Equal(ids, []string{"ACoAA1", "ACoAA2"}) {
		t.Errorf("ids=%v want [ACoAA1 ACoAA2]", ids)
	}
	if gotPath != "/api/v1/users/relations" {
		t.Errorf("path=%q want /api/v1/users/relations", gotPath)
	}
	if !strings.Contains(gotQuery, "account_id=acc_1") {
		t.Errorf("query=%q want account_id=acc_1", gotQuery)
	}
}

// TestAccountConnections_Paginates walks every page via the envelope cursor so
// a multi-page account's connections come back complete (issue #7).
func TestAccountConnections_Paginates(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		if cursor == "" {
			_, _ = w.Write([]byte(`{"object":"UserRelationsList","items":[{"member_id":"ACoAA1"}],"cursor":"page2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"UserRelationsList","items":[{"member_id":"ACoAA2"},{"member_id":"ACoAA3"}],"cursor":null}`))
	}))
	t.Cleanup(srv.Close)

	m := unipile.New("key", srv.URL, nil, srv.Client())
	m.SetBaseURL(srv.URL)

	ids, err := m.AccountConnections(context.Background(), "acc_1")
	if err != nil {
		t.Fatalf("AccountConnections: %v", err)
	}
	if !slices.Equal(ids, []string{"ACoAA1", "ACoAA2", "ACoAA3"}) {
		t.Errorf("ids=%v want [ACoAA1 ACoAA2 ACoAA3] across pages", ids)
	}
	if !slices.Equal(cursors, []string{"", "page2"}) {
		t.Errorf("cursors=%v want [\"\" \"page2\"] (followed the envelope cursor)", cursors)
	}
}

func TestAccountConnections_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.AccountConnections(context.Background(), "acc_1"); err != unipile.ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}
