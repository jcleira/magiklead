package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

type ClerkHandler struct {
	queries *repository.Queries
}

func NewClerkHandler(q *repository.Queries) *ClerkHandler {
	return &ClerkHandler{queries: q}
}

// HandleWebhook handles POST /api/v1/webhooks/clerk
func (h *ClerkHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Verify Svix webhook signature
	// For MVP, trust the payload

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

	// Check idempotency
	_, err := h.queries.GetUserByClerkID(r.Context(), userData.ID)
	if err == nil {
		return // Already exists
	}

	// Create user
	user, err := h.queries.CreateUser(r.Context(), repository.CreateUserParams{
		ClerkID: userData.ID,
		Email:   email,
		Name:    pgtype.Text{String: name, Valid: name != ""},
	})
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		return
	}

	// Create default tenant
	tenant, err := h.queries.CreateTenant(r.Context(), repository.CreateTenantParams{
		Name: name + "'s Workspace",
	})
	if err != nil {
		log.Printf("Failed to create tenant: %v", err)
		return
	}

	// Link user to tenant
	h.queries.CreateUserTenant(r.Context(), repository.CreateUserTenantParams{
		UserID:   user.ID,
		TenantID: tenant.ID,
		Role:     pgtype.Text{String: "owner", Valid: true},
	})

	// Create free subscription
	freeLimits := PlanLimits["free"]
	h.queries.CreateSubscription(r.Context(), repository.CreateSubscriptionParams{
		TenantID:       tenant.ID,
		Plan:           "free",
		LeadsLimit:     freeLimits.Leads,
		SequencesLimit: freeLimits.Sequences,
	})

	log.Printf("Created user %s, tenant %s", user.ID, tenant.ID)
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
