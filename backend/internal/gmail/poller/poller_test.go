package poller

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
)

// stubGmail boots an httptest server that answers the endpoints the
// poller touches: oauth2 refresh, profile (for cursor bootstrap),
// history.list (for the poll itself), and messages.get (for per-message
// header fetches). Each handler is overridable per-test so failure
// paths can be exercised without rebuilding the whole mux.
type stubGmail struct {
	server          *httptest.Server
	cfg             *oauth2.Config
	tokenHandler    func(w http.ResponseWriter, r *http.Request)
	profileHandler  func(w http.ResponseWriter, r *http.Request)
	historyHandler  func(w http.ResponseWriter, r *http.Request)
	messageHandlers map[string]func(w http.ResponseWriter, r *http.Request)
	historyCalls    int
	profileCalls    int
	messageCalls    int
}

func newStubGmail(t *testing.T) *stubGmail {
	t.Helper()
	s := &stubGmail{messageHandlers: map[string]func(w http.ResponseWriter, r *http.Request){}}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if s.tokenHandler != nil {
			s.tokenHandler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"refreshed","token_type":"Bearer","refresh_token":"rt","expires_in":3600}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		s.profileCalls++
		if s.profileHandler != nil {
			s.profileHandler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"sender@example.com","historyId":"100"}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		s.historyCalls++
		if s.historyHandler != nil {
			s.historyHandler(w, r)
			return
		}
		// Default: no new history.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"historyId":"100"}`))
	})
	mux.HandleFunc("/gmail/v1/users/me/messages/", func(w http.ResponseWriter, r *http.Request) {
		s.messageCalls++
		// Last path segment is the message-id.
		parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
		msgID := parts[len(parts)-1]
		if h, ok := s.messageHandlers[msgID]; ok {
			h(w, r)
			return
		}
		// Default: minimal payload, no relevant headers.
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"threadId":"thread-1","payload":{"headers":[]}}`, msgID)
	})

	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)

	s.cfg = &oauth2.Config{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Endpoint: oauth2.Endpoint{
			TokenURL: s.server.URL + "/oauth2/token",
			AuthURL:  s.server.URL + "/oauth2/auth",
		},
		Scopes: []string{
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/gmail.send",
		},
	}
	return s
}

func freshAccount(cursor string) Account {
	return Account{
		Email:        "sender@example.com",
		AccessToken:  "at",
		RefreshToken: "rt",
		TokenExpiry:  time.Now().Add(time.Hour),
		Cursor:       cursor,
	}
}

// messageResp builds the metadata response Gmail returns for
// users.messages.get?format=metadata. headers is an ordered list of
// (name, value) pairs; mimeType is what the poller checks for DSN
// classification.
func messageResp(id, threadID, mimeType string, headers ...[2]string) string {
	hs := make([]map[string]string, 0, len(headers))
	for _, h := range headers {
		hs = append(hs, map[string]string{"name": h[0], "value": h[1]})
	}
	payload := map[string]any{
		"id":       id,
		"threadId": threadID,
		"payload": map[string]any{
			"mimeType": mimeType,
			"headers":  hs,
		},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// TestTick_ReplyClassified is the tracer: stubbed history returns one
// new inbound message whose In-Reply-To matches a tracked send. The
// poller resolves the In-Reply-To via the SentLookup callback and
// emits a ClassifiedEvent{Kind: reply, CampaignLeadID: <known>} with
// the cursor advanced to the historyId Gmail returned.
func TestTick_ReplyClassified(t *testing.T) {
	stub := newStubGmail(t)

	const sentMsgID = "<sent-by-us@mail.gmail.com>"
	const inboundMsgID = "msg-inbound-1"
	knownLead := uuid.New()

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		// startHistoryId echoes the caller's cursor.
		if got := r.URL.Query().Get("startHistoryId"); got != "100" {
			t.Errorf("startHistoryId=%q want 100", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId": "205",
			"history": [
				{"id":"1001","messagesAdded":[{"message":{"id":%q,"threadId":"thread-1","labelIds":["INBOX"]}}]}
			]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(
			inboundMsgID, "thread-1", "text/plain",
			[2]string{"From", "lead@example.com"},
			[2]string{"To", "sender@example.com"},
			[2]string{"In-Reply-To", sentMsgID},
		)))
	}

	lookups := 0
	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		lookups++
		if msgID != sentMsgID {
			return uuid.Nil, false, nil
		}
		return knownLead, true, nil
	}

	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	newCursor, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if newCursor != "205" {
		t.Errorf("newCursor=%q want 205", newCursor)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != KindReply {
		t.Errorf("Kind=%q want reply", ev.Kind)
	}
	if ev.InboundMessageID != inboundMsgID {
		t.Errorf("InboundMessageID=%q want %q", ev.InboundMessageID, inboundMsgID)
	}
	if ev.InReplyTo != sentMsgID {
		t.Errorf("InReplyTo=%q want %q", ev.InReplyTo, sentMsgID)
	}
	if ev.CampaignLeadID != knownLead {
		t.Errorf("CampaignLeadID=%v want %v", ev.CampaignLeadID, knownLead)
	}
	if lookups == 0 {
		t.Errorf("SentLookup never invoked")
	}
}

// dsnMessageResp builds a Gmail messages.get response for a full-
// format fetch of a DSN. It mirrors the structure real Gmail returns
// for bounces: outer multipart/report payload with an outer
// In-Reply-To header pointing back at our send, plus a child
// message/delivery-status part whose base64url-encoded Body.Data
// contains the RFC 3464 Status: + Final-Recipient: fields the
// classifier parses.
func dsnMessageResp(id, threadID, inReplyTo, statusCode, recipient string) string {
	dsnBody := ""
	if recipient != "" {
		dsnBody += "Final-Recipient: rfc822; " + recipient + "\r\n"
	}
	dsnBody += "Action: failed\r\n"
	if statusCode != "" {
		dsnBody += "Status: " + statusCode + "\r\n"
	}
	encoded := base64.URLEncoding.EncodeToString([]byte(dsnBody))

	headers := []map[string]string{
		{"name": "From", "value": "mailer-daemon@example.com"},
		{"name": "Subject", "value": "Delivery Status Notification (Failure)"},
		{"name": "Content-Type", "value": "multipart/report; report-type=delivery-status"},
	}
	if inReplyTo != "" {
		headers = append(headers, map[string]string{"name": "In-Reply-To", "value": inReplyTo})
	}

	payload := map[string]any{
		"id":       id,
		"threadId": threadID,
		"payload": map[string]any{
			"mimeType": "multipart/report; report-type=delivery-status",
			"headers":  headers,
			"parts": []map[string]any{
				{
					"mimeType": "text/plain",
					"body":     map[string]string{"data": base64.URLEncoding.EncodeToString([]byte("The mailbox could not be found."))},
				},
				{
					"mimeType": "message/delivery-status",
					"body":     map[string]string{"data": encoded},
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

// TestTick_HardBounceClassified — a multipart/report DSN with a 5.x.x
// Status: line surfaces as KindBounceHard with DSNStatusCode +
// OriginalRecipient populated and CampaignLeadID resolved through
// SentLookup. This is the tracer for issue #5: the classifier is now
// content-aware, not just content-type-aware.
func TestTick_HardBounceClassified(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-bounce-hard-1"
	const sentMsgID = "<our-hard-sent@mail.gmail.com>"
	const bouncedRecipient = "deadletter@example.com"
	knownLead := uuid.New()

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"210",
			"history":[{"id":"1002","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dsnMessageResp(inboundMsgID, "t", sentMsgID, "5.1.1", bouncedRecipient)))
	}

	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		if msgID == sentMsgID {
			return knownLead, true, nil
		}
		return uuid.Nil, false, nil
	}
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != KindBounceHard {
		t.Errorf("Kind=%q want %q", ev.Kind, KindBounceHard)
	}
	if ev.DSNStatusCode != "5.1.1" {
		t.Errorf("DSNStatusCode=%q want 5.1.1", ev.DSNStatusCode)
	}
	if ev.OriginalRecipient != bouncedRecipient {
		t.Errorf("OriginalRecipient=%q want %q", ev.OriginalRecipient, bouncedRecipient)
	}
	if ev.CampaignLeadID != knownLead {
		t.Errorf("CampaignLeadID=%v want %v", ev.CampaignLeadID, knownLead)
	}
	if ev.InReplyTo != sentMsgID {
		t.Errorf("InReplyTo=%q want %q", ev.InReplyTo, sentMsgID)
	}
}

// TestTick_SoftBounceClassified — a 4.x.x DSN routes to KindBounceSoft.
// The worker's RecordSoftBounce will increment the consecutive-soft-
// bounce counter; threshold enforcement is in the suppression module
// from #2 (this slice just routes).
func TestTick_SoftBounceClassified(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-bounce-soft-1"
	const sentMsgID = "<our-soft-sent@mail.gmail.com>"
	const bouncedRecipient = "mailbox-full@example.com"
	knownLead := uuid.New()

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"211",
			"history":[{"id":"1003","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dsnMessageResp(inboundMsgID, "t", sentMsgID, "4.2.2", bouncedRecipient)))
	}
	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		if msgID == sentMsgID {
			return knownLead, true, nil
		}
		return uuid.Nil, false, nil
	}
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != KindBounceSoft {
		t.Errorf("Kind=%q want %q", ev.Kind, KindBounceSoft)
	}
	if ev.DSNStatusCode != "4.2.2" {
		t.Errorf("DSNStatusCode=%q want 4.2.2", ev.DSNStatusCode)
	}
	if ev.CampaignLeadID != knownLead {
		t.Errorf("CampaignLeadID=%v want %v", ev.CampaignLeadID, knownLead)
	}
}

// TestTick_BounceWithoutStatusCode — a DSN whose delivery-status
// part lacks a Status: line is dropped (no event emitted). The
// classifier logs a warn for operator visibility because a real
// undeliverable should not silently disappear from observability;
// this asserts the routing decision.
func TestTick_BounceWithoutStatusCode(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-bounce-noStatus"
	const sentMsgID = "<our-sent@x>"

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"212",
			"history":[{"id":"1004","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// statusCode empty → Status: line omitted.
		_, _ = w.Write([]byte(dsnMessageResp(inboundMsgID, "t", sentMsgID, "", "x@example.com")))
	}
	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		if msgID == sentMsgID {
			return uuid.New(), true, nil
		}
		return uuid.Nil, false, nil
	}
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0 (DSN without Status: must be dropped)", len(events))
	}
}

// TestTick_BounceForUnsentMessage — DSN references a message-id we
// never sent (SentLookup says no). Treated as unrelated and dropped
// — someone else's bounce landed in the connected mailbox; not ours
// to record.
func TestTick_BounceForUnsentMessage(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-bounce-stranger"

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"213",
			"history":[{"id":"1005","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dsnMessageResp(inboundMsgID, "t", "<stranger@elsewhere>", "5.1.1", "anyone@x.com")))
	}
	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0 (bounce for unsent message dropped)", len(events))
	}
}

// TestTick_UnrelatedIgnored — an inbound message with no In-Reply-To
// and no DSN content-type is unrelated. The poller emits nothing for
// it; if it were emitted with Kind=unrelated the worker would still
// have to skip it, so dropping at the source keeps callers simpler.
func TestTick_UnrelatedIgnored(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-unrelated-1"

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"215",
			"history":[{"id":"1003","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(
			inboundMsgID, "t", "text/plain",
			[2]string{"From", "newsletter@example.com"},
			[2]string{"Subject", "Newsletter — May edition"},
		)))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	newCursor, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if newCursor != "215" {
		t.Errorf("newCursor=%q want 215", newCursor)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0 (unrelated dropped at source)", len(events))
	}
}

// TestTick_ReplyButUnknownInReplyTo — the inbound carries an
// In-Reply-To, but SentLookup says we never sent that message-id.
// Treated as unrelated (someone replied to a thread we weren't on).
func TestTick_ReplyButUnknownInReplyTo(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-stranger-1"

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"220",
			"history":[{"id":"1004","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(
			inboundMsgID, "t", "text/plain",
			[2]string{"In-Reply-To", "<stranger@elsewhere>"},
		)))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0", len(events))
	}
}

// TestTick_ReplyViaReferences — Gmail clients sometimes drop
// In-Reply-To but keep References (space-separated chain). The poller
// must match against any element of References too.
func TestTick_ReplyViaReferences(t *testing.T) {
	stub := newStubGmail(t)
	const inboundMsgID = "msg-refs-1"
	const ourMsgID = "<our@send.com>"
	knownLead := uuid.New()

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"230",
			"history":[{"id":"1005","messagesAdded":[{"message":{"id":%q,"threadId":"t","labelIds":["INBOX"]}}]}]
		}`, inboundMsgID)
	}
	stub.messageHandlers[inboundMsgID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(
			inboundMsgID, "t", "text/plain",
			[2]string{"References", "<unrelated@x> " + ourMsgID + " <other@y>"},
		)))
	}

	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		if msgID == ourMsgID {
			return knownLead, true, nil
		}
		return uuid.Nil, false, nil
	}
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(events) != 1 || events[0].Kind != KindReply {
		t.Fatalf("events=%+v want 1 reply", events)
	}
	if events[0].CampaignLeadID != knownLead {
		t.Errorf("CampaignLeadID=%v want %v", events[0].CampaignLeadID, knownLead)
	}
	if events[0].InReplyTo != ourMsgID {
		t.Errorf("InReplyTo=%q want %q", events[0].InReplyTo, ourMsgID)
	}
}

// TestTick_FirstPoll_BootstrapsCursor — on the very first poll the
// cursor is empty. Gmail can't run history.list without a starting
// point, so the poller fetches users.getProfile to read the baseline
// historyId, returns it as the new cursor, and emits no events
// (everything that happened before bootstrap is intentionally not
// surfaced — we'd otherwise replay months of inbox on connect).
func TestTick_FirstPoll_BootstrapsCursor(t *testing.T) {
	stub := newStubGmail(t)
	stub.profileHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"sender@example.com","historyId":"500"}`))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	newCursor, events, err := p.Tick(context.Background(), freshAccount(""))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if newCursor != "500" {
		t.Errorf("newCursor=%q want 500", newCursor)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0 on first poll", len(events))
	}
	if stub.profileCalls != 1 {
		t.Errorf("profileCalls=%d want 1", stub.profileCalls)
	}
	if stub.historyCalls != 0 {
		t.Errorf("historyCalls=%d want 0 (no cursor → no history.list)", stub.historyCalls)
	}
}

// TestTick_HistoryRotated — Gmail rotates history-ids after ~7 days of
// inactivity and returns 404 on history.list with an old cursor. The
// poller must re-bootstrap from profile, return the fresh cursor,
// and emit no events (we've lost the gap; next poll picks up cleanly).
func TestTick_HistoryRotated(t *testing.T) {
	stub := newStubGmail(t)
	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"Requested entity was not found.","errors":[{"reason":"notFound"}]}}`))
	}
	stub.profileHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emailAddress":"sender@example.com","historyId":"999"}`))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	newCursor, events, err := p.Tick(context.Background(), freshAccount("oldcursor"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if newCursor != "999" {
		t.Errorf("newCursor=%q want 999", newCursor)
	}
	if len(events) != 0 {
		t.Errorf("events=%d want 0 after rotation", len(events))
	}
	if stub.profileCalls == 0 {
		t.Errorf("profile not consulted after rotation")
	}
}

// TestTick_TokenExpired — refresh fails (refresh token revoked). The
// poller surfaces ErrTokenExpired so the worker can mark the account
// for reconnect, without advancing the cursor.
func TestTick_TokenExpired(t *testing.T) {
	stub := newStubGmail(t)
	stub.tokenHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	acc := freshAccount("100")
	acc.TokenExpiry = time.Now().Add(-time.Hour) // force a refresh attempt
	_, _, err := p.Tick(context.Background(), acc)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("err=%v want ErrTokenExpired", err)
	}
}

// TestTick_RateLimited — 429 on history.list. Worker treats this as
// transient: returns ErrRateLimited, cursor not advanced, next tick
// retries.
func TestTick_RateLimited(t *testing.T) {
	stub := newStubGmail(t)
	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"Rate limit"}}`))
	}

	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	_, _, err := p.Tick(context.Background(), freshAccount("100"))
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("err=%v want ErrRateLimited", err)
	}
}

// TestTick_NetworkError — the transport itself fails (server closed).
// Surfaced as a generic wrapped error so the worker logs-and-retries
// without misclassifying as a permanent failure.
func TestTick_NetworkError(t *testing.T) {
	stub := newStubGmail(t)
	stub.server.Close()
	lookup := func(context.Context, string) (uuid.UUID, bool, error) { return uuid.Nil, false, nil }
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)

	_, _, err := p.Tick(context.Background(), freshAccount("100"))
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrRateLimited) {
		t.Errorf("network error misclassified: %v", err)
	}
}

// TestTick_MultipleMessages_MixedKinds — one tick can surface several
// messages at once. The poller must emit one event per classifiable
// message (reply or bounce), drop the rest, and advance the cursor
// past the whole batch.
func TestTick_MultipleMessages_MixedKinds(t *testing.T) {
	stub := newStubGmail(t)
	const replyID, bounceID, unrelatedID = "m-r", "m-b", "m-u"
	const sentMsgID = "<our-sent@x>"
	knownLead := uuid.New()

	stub.historyHandler = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{
			"historyId":"600",
			"history":[
				{"id":"2001","messagesAdded":[{"message":{"id":%q,"labelIds":["INBOX"]}}]},
				{"id":"2002","messagesAdded":[{"message":{"id":%q,"labelIds":["INBOX"]}}]},
				{"id":"2003","messagesAdded":[{"message":{"id":%q,"labelIds":["INBOX"]}}]}
			]
		}`, replyID, bounceID, unrelatedID)
	}
	stub.messageHandlers[replyID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(replyID, "t", "text/plain", [2]string{"In-Reply-To", sentMsgID})))
	}
	stub.messageHandlers[bounceID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dsnMessageResp(bounceID, "t", sentMsgID, "5.1.1", "dead@example.com")))
	}
	stub.messageHandlers[unrelatedID] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(messageResp(unrelatedID, "t", "text/plain")))
	}

	lookup := func(_ context.Context, msgID string) (uuid.UUID, bool, error) {
		if msgID == sentMsgID {
			return knownLead, true, nil
		}
		return uuid.Nil, false, nil
	}
	p := NewForTest(stub.cfg, stub.server.URL+"/", lookup)
	newCursor, events, err := p.Tick(context.Background(), freshAccount("100"))
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if newCursor != "600" {
		t.Errorf("newCursor=%q want 600", newCursor)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d want 2 (reply+bounce, unrelated dropped)", len(events))
	}
	kinds := map[Kind]int{}
	for _, e := range events {
		kinds[e.Kind]++
	}
	if kinds[KindReply] != 1 || kinds[KindBounceHard] != 1 {
		t.Errorf("kinds=%v want reply=1 bounce-hard=1", kinds)
	}
}
