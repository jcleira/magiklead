package unipile_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// webhookListBody is the frozen GET /api/v1/webhooks envelope, captured live
// from the workspace on 2026-07-24. It pins the three source names the rail
// depends on and their event selectors — account_status carries none, users
// carries new_relation, messaging carries message_received. The per-source
// `data` key lists are trimmed; nothing reads them.
const webhookListBody = `{
  "object": "WebhookList",
  "items": [
    {
      "object": "Webhook",
      "id": "fdAhDzUxQbO-752ebd3Saw",
      "name": "magiklead-mvp-messaging",
      "enabled": true,
      "format": "json",
      "request_url": "https://tunnel.example.com/api/v1/webhooks/unipile",
      "headers": [{"key": "Unipile-Auth", "value": "secret"}],
      "source": "messaging",
      "events": ["message_received"]
    },
    {
      "object": "Webhook",
      "id": "iwdjoB58Q7qIZYQDLPHbcg",
      "name": "magiklead-mvp-users",
      "enabled": true,
      "format": "json",
      "request_url": "https://tunnel.example.com/api/v1/webhooks/unipile",
      "headers": [{"key": "Unipile-Auth", "value": "secret"}],
      "source": "users",
      "events": ["new_relation"]
    },
    {
      "object": "Webhook",
      "id": "lv7ahylIQ7iGH7u6wK8OUQ",
      "name": "magiklead-mvp-account_status",
      "enabled": false,
      "format": "json",
      "request_url": "https://tunnel.example.com/api/v1/webhooks/unipile",
      "headers": [{"key": "Unipile-Auth", "value": "secret"}],
      "source": "account_status",
      "events": []
    }
  ],
  "cursor": null
}`

func TestListWebhooks_ParsesTheWebhookListEnvelope(t *testing.T) {
	s := newStub(t, 200, webhookListBody)
	m := unipile.New("key", s.URL, nil, s.Client())

	hooks, err := m.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(hooks) != 3 {
		t.Fatalf("got %d registrations, want 3", len(hooks))
	}
	if s.method != http.MethodGet || s.path != "/api/v1/webhooks" {
		t.Errorf("request = %s %s, want GET /api/v1/webhooks", s.method, s.path)
	}

	first := hooks[0]
	if first.ID != "fdAhDzUxQbO-752ebd3Saw" || first.Source != "messaging" ||
		first.Name != "magiklead-mvp-messaging" ||
		first.RequestURL != "https://tunnel.example.com/api/v1/webhooks/unipile" {
		t.Errorf("first registration = %+v, want the messaging row verbatim", first)
	}
	if !slices.Equal(first.Events, []string{"message_received"}) {
		t.Errorf("messaging events = %v, want [message_received]", first.Events)
	}
	if !first.Enabled {
		t.Error("messaging registration should read as enabled")
	}
	if !slices.Equal(hooks[1].Events, []string{"new_relation"}) {
		t.Errorf("users events = %v, want [new_relation]", hooks[1].Events)
	}
	if len(hooks[2].Events) != 0 {
		t.Errorf("account_status events = %v, want none", hooks[2].Events)
	}
	if hooks[2].Enabled {
		t.Error("a disabled registration must not read as enabled — that is the signal the operator acts on")
	}
}

func TestCreateWebhook_SendsSpecAndReturnsAssignedID(t *testing.T) {
	s := newStub(t, 201, `{"object":"Webhook","id":"wh_new","source":"messaging",
	  "name":"magiklead-smoke-messaging","request_url":"https://smoke.example.com/api/v1/webhooks/unipile",
	  "events":["message_received"],"enabled":true}`)
	m := unipile.New("key", s.URL, nil, s.Client())

	got, err := m.CreateWebhook(context.Background(), unipile.WebhookSpec{
		Source:     "messaging",
		Name:       "magiklead-smoke-messaging",
		RequestURL: "https://smoke.example.com/api/v1/webhooks/unipile",
		Events:     []string{"message_received"},
		Headers:    []unipile.WebhookHeader{{Key: "Unipile-Auth", Value: "the-secret"}},
	})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if got.ID != "wh_new" {
		t.Errorf("created id = %q, want wh_new", got.ID)
	}
	if s.method != http.MethodPost || s.path != "/api/v1/webhooks" {
		t.Errorf("request = %s %s, want POST /api/v1/webhooks", s.method, s.path)
	}

	var sent map[string]any
	if err := json.Unmarshal(s.lastBody, &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if sent["source"] != "messaging" || sent["name"] != "magiklead-smoke-messaging" ||
		sent["request_url"] != "https://smoke.example.com/api/v1/webhooks/unipile" {
		t.Errorf("sent body = %v, want the spec verbatim", sent)
	}
	hdrs, _ := sent["headers"].([]any)
	if len(hdrs) != 1 {
		t.Fatalf("sent headers = %v, want the single Unipile-Auth header", sent["headers"])
	}
	h, _ := hdrs[0].(map[string]any)
	if h["key"] != "Unipile-Auth" || h["value"] != "the-secret" {
		t.Errorf("sent auth header = %v, want Unipile-Auth=the-secret", h)
	}
}

// TestCreateWebhook_OmitsEmptyEvents guards the account_status shape: that
// source takes no event selector, and sending an empty array where Unipile
// expects the key absent is the kind of thing it answers 400 to.
func TestCreateWebhook_OmitsEmptyEvents(t *testing.T) {
	s := newStub(t, 201, `{"object":"Webhook","id":"wh_status","source":"account_status"}`)
	m := unipile.New("key", s.URL, nil, s.Client())

	if _, err := m.CreateWebhook(context.Background(), unipile.WebhookSpec{
		Source:     "account_status",
		Name:       "magiklead-smoke-account_status",
		RequestURL: "https://smoke.example.com/api/v1/webhooks/unipile",
	}); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(s.lastBody, &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	if _, present := sent["events"]; present {
		t.Errorf("account_status body carried an events key (%v); it must be omitted", sent["events"])
	}
}

func TestDeleteWebhook_TargetsTheID(t *testing.T) {
	s := newStub(t, 200, `{"object":"WebhookDeleted"}`)
	m := unipile.New("key", s.URL, nil, s.Client())

	if err := m.DeleteWebhook(context.Background(), "wh_stale"); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	if s.method != http.MethodDelete || s.path != "/api/v1/webhooks/wh_stale" {
		t.Errorf("request = %s %s, want DELETE /api/v1/webhooks/wh_stale", s.method, s.path)
	}
}

// TestWebhookCRUD_UnconfiguredModule covers the path the CLI's own tests
// cannot reach: a process with no UNIPILE_API_KEY must refuse cleanly rather
// than firing keyless calls at Unipile.
func TestWebhookCRUD_UnconfiguredModule(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	ctx := context.Background()

	if _, err := m.ListWebhooks(ctx); !errors.Is(err, unipile.ErrNotConfigured) {
		t.Errorf("ListWebhooks error = %v, want ErrNotConfigured", err)
	}
	if _, err := m.CreateWebhook(ctx, unipile.WebhookSpec{Source: "messaging"}); !errors.Is(err, unipile.ErrNotConfigured) {
		t.Errorf("CreateWebhook error = %v, want ErrNotConfigured", err)
	}
	if err := m.DeleteWebhook(ctx, "wh_1"); !errors.Is(err, unipile.ErrNotConfigured) {
		t.Errorf("DeleteWebhook error = %v, want ErrNotConfigured", err)
	}
}

func TestWebhookCRUD_ClassifiesUpstreamErrors(t *testing.T) {
	t.Run("unauthorized list", func(t *testing.T) {
		s := newStub(t, 401, `{"detail":"invalid api key"}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		if _, err := m.ListWebhooks(context.Background()); !errors.Is(err, unipile.ErrUnauthorized) {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("rejected create", func(t *testing.T) {
		s := newStub(t, 400, `{"detail":"unknown source"}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		_, err := m.CreateWebhook(context.Background(), unipile.WebhookSpec{Source: "nope"})
		if !errors.Is(err, unipile.ErrUpstream) {
			t.Errorf("error = %v, want ErrUpstream", err)
		}
	})

	// A 2xx that carries no id would otherwise look like a success and leave
	// the operator believing a registration exists when none does.
	t.Run("create response without id", func(t *testing.T) {
		s := newStub(t, 201, `{"object":"Webhook"}`)
		m := unipile.New("key", s.URL, nil, s.Client())
		_, err := m.CreateWebhook(context.Background(), unipile.WebhookSpec{Source: "messaging"})
		if !errors.Is(err, unipile.ErrMalformed) {
			t.Errorf("error = %v, want ErrMalformed", err)
		}
	})
}
