package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	svix "github.com/svix/svix-webhooks/go"

	"github.com/jcleira/magiklead/backend/internal/bootstrap"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

type ClerkHandler struct {
	queries *repository.Queries
	pool    *pgxpool.Pool
	webhook *svix.Webhook
}

// NewClerkHandler builds the handler. webhookSecret is the
// CLERK_WEBHOOK_SECRET (format: "whsec_<base64>") used to verify Svix
// signatures on /api/v1/webhooks/clerk. An empty secret causes the
// constructor to return a handler that rejects every webhook — this
// is intentional: the app must not silently accept unverified payloads
// in production. pool is the same pgxpool the rest of the api shares;
// the user.created branch hands it to bootstrap.Bootstrap so a single
// transaction provisions the user/tenant/role/subscription quartet.
func NewClerkHandler(q *repository.Queries, pool *pgxpool.Pool, webhookSecret string) *ClerkHandler {
	h := &ClerkHandler{queries: q, pool: pool}
	if webhookSecret != "" {
		wh, err := svix.NewWebhook(webhookSecret)
		if err != nil {
			// Log loud and continue — `HandleWebhook` already returns
			// 503 when h.webhook is nil, so a bad secret degrades the
			// webhook path without crashing the api. Production runs
			// will trip the 503 immediately on first delivery and the
			// log line names the cause.
			log.Printf("CLERK_WEBHOOK_SECRET is set but invalid (%v); webhook endpoint will return 503 until corrected", err)
			return h
		}
		h.webhook = wh
	}
	return h
}

// HandleWebhook handles POST /api/v1/webhooks/clerk
func (h *ClerkHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.webhook == nil {
		log.Print("clerk webhook rejected: signature verification not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := h.webhook.Verify(body, r.Header); err != nil {
		log.Printf("clerk webhook signature verification failed: %v", err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var event struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Failed to parse clerk webhook: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case "user.created":
		h.handleUserCreated(r, event.Data)
	case "user.updated":
		h.handleUserUpdated(r, event.Data)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *ClerkHandler) handleUserCreated(r *http.Request, data json.RawMessage) {
	var userData struct {
		ID             string `json:"id"`
		EmailAddresses []struct {
			EmailAddress string `json:"email_address"`
		} `json:"email_addresses"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.Unmarshal(data, &userData); err != nil {
		log.Printf("Failed to parse user data: %v", err)
		return
	}

	email := ""
	if len(userData.EmailAddresses) > 0 {
		email = userData.EmailAddresses[0].EmailAddress
	}
	name := userData.FirstName
	if userData.LastName != "" {
		name += " " + userData.LastName
	}

	tenantID, err := bootstrap.Bootstrap(r.Context(), h.pool, userData.ID, email, name)
	if err != nil {
		log.Printf("clerk webhook: bootstrap failed for %s: %v", userData.ID, err)
		return
	}
	log.Printf("clerk webhook: bootstrapped clerk_id=%s tenant_id=%s", userData.ID, tenantID)
}

func (h *ClerkHandler) handleUserUpdated(r *http.Request, data json.RawMessage) {
	var userData struct {
		ID             string `json:"id"`
		EmailAddresses []struct {
			EmailAddress string `json:"email_address"`
		} `json:"email_addresses"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.Unmarshal(data, &userData); err != nil {
		return
	}

	email := ""
	if len(userData.EmailAddresses) > 0 {
		email = userData.EmailAddresses[0].EmailAddress
	}
	name := userData.FirstName
	if userData.LastName != "" {
		name += " " + userData.LastName
	}

	h.queries.UpdateUser(r.Context(), repository.UpdateUserParams{
		ClerkID: userData.ID,
		Email:   email,
		Name:    pgtype.Text{String: name, Valid: name != ""},
	})
}
