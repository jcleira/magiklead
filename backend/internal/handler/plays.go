package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/ai"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type PlayHandler struct {
	ai      *ai.Client
	queries *repository.Queries
}

func NewPlayHandler(aiClient *ai.Client, q *repository.Queries) *PlayHandler {
	return &PlayHandler{ai: aiClient, queries: q}
}

// Generate handles POST /api/v1/plays/generate
func (h *PlayHandler) Generate(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	if tenantID == uuid.Nil {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}

	tenant, err := h.queries.GetTenant(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	var profile ai.BusinessProfile
	if err := json.Unmarshal(tenant.BusinessProfile, &profile); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "no_profile", Message: "Analyze your website first"})
		return
	}

	plays, err := h.ai.GeneratePlays(r.Context(), &profile)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "ai_error", Message: err.Error()})
		return
	}

	type playResponse struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Titles      []string `json:"titles"`
		Industry    string   `json:"industry"`
		CompanySize string   `json:"company_size"`
		Signal      string   `json:"signal"`
		Channels    []string `json:"channels"`
	}

	var result []playResponse
	for _, p := range plays {
		icpJSON, _ := json.Marshal(p.ICP)
		queryJSON, _ := json.Marshal(p.SearchQuery)

		dbPlay, err := h.queries.CreatePlay(r.Context(), repository.CreatePlayParams{
			TenantID:    pgUUID(tenantID),
			Name:        p.Name,
			Description: pgtype.Text{String: p.Description, Valid: true},
			Icp:         icpJSON,
			SearchQuery: queryJSON,
			Channels:    p.Channels,
			Status:      pgtype.Text{String: "active", Valid: true},
		})
		if err != nil {
			continue
		}

		resp := playResponse{
			ID:          fmt.Sprintf("%x", dbPlay.ID.Bytes),
			Name:        dbPlay.Name,
			Description: p.Description,
			Titles:      p.ICP.Titles,
			Industry:    strings.Join(p.ICP.Industry, ", "),
			CompanySize: p.ICP.CompanySize,
			Signal:      p.Signal,
			Channels:    dbPlay.Channels,
		}
		result = append(result, resp)
	}

	apierr.WriteJSON(w, http.StatusOK, result)
}

// List handles GET /api/v1/plays
func (h *PlayHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	plays, err := h.queries.ListPlays(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, plays)
}

// Get handles GET /api/v1/plays/:id
func (h *PlayHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	play, err := h.queries.GetPlay(r.Context(), repository.GetPlayParams{ID: pgUUID(id), TenantID: pgUUID(tenantID)})
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, play)
}

// Delete handles DELETE /api/v1/plays/:id
func (h *PlayHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	h.queries.DeletePlay(r.Context(), repository.DeletePlayParams{ID: pgUUID(id), TenantID: pgUUID(tenantID)})
	w.WriteHeader(http.StatusNoContent)
}
