package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/ai"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type WebsiteHandler struct {
	ai      *ai.Client
	queries *repository.Queries
}

func NewWebsiteHandler(aiClient *ai.Client, q *repository.Queries) *WebsiteHandler {
	return &WebsiteHandler{ai: aiClient, queries: q}
}

// Analyze handles POST /api/v1/websites/analyze
func (h *WebsiteHandler) Analyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	if req.URL == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "url is required"})
		return
	}

	profile, err := h.ai.AnalyzeWebsite(r.Context(), req.URL)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "ai_error", Message: err.Error()})
		return
	}

	tenantID := getTenantID(r.Context())
	if tenantID != uuid.Nil {
		profileJSON, _ := json.Marshal(profile)
		h.queries.UpdateTenantProfile(r.Context(), repository.UpdateTenantProfileParams{
			ID:              pgUUID(tenantID),
			BusinessProfile: profileJSON,
		})
	}

	apierr.WriteJSON(w, http.StatusOK, profile)
}
