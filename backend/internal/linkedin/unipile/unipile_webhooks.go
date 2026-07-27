package unipile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Webhook is one registration at Unipile — the app's inbound callback for a
// given source. Unipile owns the id; everything else is what we sent when the
// registration was created. Only the fields the registration CLI reasons about
// are kept: Unipile also returns a `data` key list (the payload fields the
// source declares) that no caller needs.
type Webhook struct {
	// ID is Unipile's identifier, the DELETE key.
	ID string
	// Source is the event family: account_status, users, or messaging.
	Source string
	// Name is the free-text marker we set at creation. The CLI writes a
	// recognizable prefix into it so prune can tell our registrations from
	// any other tenant's on the same workspace.
	Name string
	// RequestURL is the endpoint Unipile POSTs to — our public
	// /api/v1/webhooks/unipile.
	RequestURL string
	// Events narrows a source to specific event types (messaging =>
	// message_received, users => new_relation). Empty for account_status,
	// which has no event selector.
	Events []string
	// Enabled reports whether Unipile is currently delivering to it.
	Enabled bool
}

// WebhookHeader is one static header Unipile echoes on every delivery. The
// registration carries `Unipile-Auth: <UNIPILE_WEBHOOK_SECRET>`, which is the
// ONLY authentication on the inbound endpoint — Unipile has no body signing,
// so VerifyAuthToken constant-time compares this value.
type WebhookHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// WebhookSpec is a registration to create.
type WebhookSpec struct {
	Source     string
	Name       string
	RequestURL string
	Events     []string
	Headers    []WebhookHeader
}

// wireWebhook is the shape Unipile returns per registration. Parsed
// permissively (like every other listing in this module) so a field Unipile
// adds never breaks the CLI.
type wireWebhook struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Name       string   `json:"name"`
	RequestURL string   `json:"request_url"`
	Events     []string `json:"events"`
	Enabled    bool     `json:"enabled"`
}

// webhookListResponse is the GET /api/v1/webhooks envelope
// ({"object":"WebhookList","items":[…],"cursor":null}).
type webhookListResponse struct {
	Items []wireWebhook `json:"items"`
}

type createWebhookRequest struct {
	Source     string          `json:"source"`
	RequestURL string          `json:"request_url"`
	Name       string          `json:"name"`
	Events     []string        `json:"events,omitempty"`
	Headers    []WebhookHeader `json:"headers,omitempty"`
}

func (w wireWebhook) toWebhook() Webhook {
	return Webhook{
		ID:         w.ID,
		Source:     w.Source,
		Name:       w.Name,
		RequestURL: w.RequestURL,
		Events:     w.Events,
		Enabled:    w.Enabled,
	}
}

// ListWebhooks returns every webhook registration on the Unipile workspace —
// ours and anyone else's. The CLI diffs it to decide what to create and
// filters it by name marker to decide what to prune.
func (m *Module) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	var out []Webhook
	if err := m.list(ctx, "/api/v1/webhooks", func(raw []byte) error {
		var resp webhookListResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("%w: %v", ErrMalformed, err)
		}
		for _, it := range resp.Items {
			out = append(out, it.toWebhook())
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateWebhook registers one source at Unipile and returns it with the id
// Unipile assigned.
func (m *Module) CreateWebhook(ctx context.Context, spec WebhookSpec) (Webhook, error) {
	if !m.Configured() {
		return Webhook{}, ErrNotConfigured
	}
	body, err := json.Marshal(createWebhookRequest{
		Source:     spec.Source,
		RequestURL: spec.RequestURL,
		Name:       spec.Name,
		Events:     spec.Events,
		Headers:    spec.Headers,
	})
	if err != nil {
		return Webhook{}, fmt.Errorf("unipile: marshal create-webhook request: %w", err)
	}

	resp, err := m.do(ctx, http.MethodPost, "/api/v1/webhooks", body)
	if err != nil {
		return Webhook{}, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return Webhook{}, err
	}
	// Unipile answers create with {"object":"WebhookCreated","webhook_id":"…"} —
	// only the new id, not the full registration the list endpoint returns
	// (wireWebhook). So read webhook_id and rebuild the Webhook from the spec we
	// just sent; there is nothing else in the body to echo.
	var created struct {
		WebhookID string `json:"webhook_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return Webhook{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if created.WebhookID == "" {
		return Webhook{}, fmt.Errorf("%w: create-webhook response had no webhook_id", ErrMalformed)
	}
	return Webhook{
		ID:         created.WebhookID,
		Source:     spec.Source,
		Name:       spec.Name,
		RequestURL: spec.RequestURL,
		Events:     spec.Events,
	}, nil
}

// DeleteWebhook removes one registration by Unipile id.
func (m *Module) DeleteWebhook(ctx context.Context, id string) error {
	if !m.Configured() {
		return ErrNotConfigured
	}
	resp, err := m.do(ctx, http.MethodDelete, "/api/v1/webhooks/"+id, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return mapStatus(resp.StatusCode, raw)
}
