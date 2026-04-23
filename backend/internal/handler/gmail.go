package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type GmailHandler struct {
	gmailSvc *gmail.Service
	queries  *repository.Queries
}

func NewGmailHandler(gmailSvc *gmail.Service, q *repository.Queries) *GmailHandler {
	return &GmailHandler{gmailSvc: gmailSvc, queries: q}
}

// AuthURL handles GET /api/v1/gmail/auth-url
func (h *GmailHandler) AuthURL(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	url := h.gmailSvc.GetAuthURL(state)
	apierr.WriteJSON(w, http.StatusOK, map[string]string{
		"url":   url,
		"state": state,
	})
}

// Callback handles GET /api/v1/gmail/callback
func (h *GmailHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "missing code parameter"})
		return
	}

	token, err := h.gmailSvc.ExchangeCode(r.Context(), code)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "oauth_error", Message: err.Error()})
		return
	}

	// Extract email from token (use userinfo endpoint or ID token)
	// For MVP, accept email from query param set by frontend
	email := r.URL.Query().Get("email")
	if email == "" {
		email = "connected@gmail.com"
	}

	tenantID := getTenantID(r.Context())
	// TODO: get real user ID from Clerk context
	userID := tenantID // placeholder

	account, err := h.gmailSvc.SaveAccount(r.Context(), pgUUID(tenantID), pgUUID(userID), email, token)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "save_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusOK, account)
}

// ListAccounts handles GET /api/v1/gmail/accounts
func (h *GmailHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	accounts, err := h.queries.ListGmailAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, accounts)
}

// DeleteAccount handles DELETE /api/v1/gmail/accounts/:id
func (h *GmailHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	h.queries.DeleteGmailAccount(r.Context(), repository.DeleteGmailAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	})
	w.WriteHeader(http.StatusNoContent)
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
