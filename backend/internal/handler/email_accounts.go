package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type EmailAccountHandler struct {
	queries *repository.Queries
}

func NewEmailAccountHandler(q *repository.Queries) *EmailAccountHandler {
	return &EmailAccountHandler{queries: q}
}

// ConnectSMTP handles POST /api/v1/email-accounts/smtp
func (h *EmailAccountHandler) ConnectSMTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email      string `json:"email"`
		SMTPHost   string `json:"smtp_host"`
		SMTPPort   int    `json:"smtp_port"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		SenderName string `json:"sender_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	if req.Email == "" || req.SMTPHost == "" || req.Password == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "email, smtp_host, and password are required"})
		return
	}

	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}
	if req.Username == "" {
		req.Username = req.Email
	}

	tenantID := getTenantID(r.Context())

	// Get user ID from clerk ID
	clerkID := middleware.GetUserClerkID(r.Context())
	user, err := h.queries.GetUserByClerkID(r.Context(), clerkID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "user_not_found", Message: "Could not find user"})
		return
	}
	userID := uuid.UUID(user.ID.Bytes)

	account, err := h.queries.CreateEmailAccount(r.Context(), repository.CreateEmailAccountParams{
		TenantID:     pgUUID(tenantID),
		UserID:       pgUUID(userID),
		Provider:     "smtp",
		Email:        req.Email,
		SmtpHost:     pgtype.Text{String: req.SMTPHost, Valid: true},
		SmtpPort:     pgtype.Int4{Int32: int32(req.SMTPPort), Valid: true},
		SmtpUsername: pgtype.Text{String: req.Username, Valid: true},
		SmtpPassword: pgtype.Text{String: req.Password, Valid: true},
		SenderName:   pgtype.Text{String: req.SenderName, Valid: req.SenderName != ""},
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "internal_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusCreated, account)
}

// List handles GET /api/v1/email-accounts
func (h *EmailAccountHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	accounts, err := h.queries.ListEmailAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, accounts)
}

// Delete handles DELETE /api/v1/email-accounts/:id
func (h *EmailAccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	h.queries.DeleteEmailAccount(r.Context(), repository.DeleteEmailAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	})
	w.WriteHeader(http.StatusNoContent)
}
