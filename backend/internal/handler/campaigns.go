package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type CampaignHandler struct {
	queries *repository.Queries
}

func NewCampaignHandler(q *repository.Queries) *CampaignHandler {
	return &CampaignHandler{queries: q}
}

// Create handles POST /api/v1/campaigns
func (h *CampaignHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayID         string `json:"play_id"`
		Name           string `json:"name"`
		GmailAccountID string `json:"gmail_account_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	tenantID := getTenantID(r.Context())
	playID, err := uuid.Parse(req.PlayID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid play_id"})
		return
	}

	var gmailID pgtype.UUID
	if req.GmailAccountID != "" {
		parsed, err := uuid.Parse(req.GmailAccountID)
		if err == nil {
			gmailID = pgtype.UUID{Bytes: parsed, Valid: true}
		}
	}

	campaign, err := h.queries.CreateCampaign(r.Context(), repository.CreateCampaignParams{
		TenantID:       pgUUID(tenantID),
		PlayID:         pgUUID(playID),
		Name:           req.Name,
		Status:         pgtype.Text{String: "draft", Valid: true},
		GmailAccountID: gmailID,
		Sequence:       []byte("[]"),
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "internal_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusCreated, campaign)
}

// List handles GET /api/v1/campaigns
func (h *CampaignHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	campaigns, err := h.queries.ListCampaigns(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	result := make([]map[string]any, len(campaigns))
	for i, c := range campaigns {
		result[i] = campaignToJSON(c)
	}
	apierr.WriteJSON(w, http.StatusOK, result)
}

// campaignToJSON converts a campaign with []byte fields to proper JSON.
func campaignToJSON(c repository.Campaign) map[string]any {
	var seq any
	if err := json.Unmarshal(c.Sequence, &seq); err != nil {
		seq = []any{}
	}
	var stats any
	if err := json.Unmarshal(c.Stats, &stats); err != nil {
		stats = map[string]any{}
	}
	var linkedinSeq any
	if len(c.LinkedinSequence) > 0 {
		json.Unmarshal(c.LinkedinSequence, &linkedinSeq)
	}

	return map[string]any{
		"id":                fmtUUID(c.ID),
		"tenant_id":         fmtUUID(c.TenantID),
		"play_id":           fmtUUID(c.PlayID),
		"name":              c.Name,
		"status":            c.Status.String,
		"gmail_account_id":  fmtUUIDNullable(c.GmailAccountID),
		"sequence":          seq,
		"linkedin_sequence": linkedinSeq,
		"stats":             stats,
		"created_at":        c.CreatedAt.Time,
	}
}

func fmtUUID(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	id := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}

func fmtUUIDNullable(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := fmtUUID(u)
	return &s
}

// Get handles GET /api/v1/campaigns/:id
func (h *CampaignHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	campaign, err := h.queries.GetCampaign(r.Context(), repository.GetCampaignParams{ID: pgUUID(id), TenantID: pgUUID(tenantID)})
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	counts, _ := h.queries.CountCampaignLeadsByStatus(r.Context(), pgUUID(id))

	apierr.WriteJSON(w, http.StatusOK, map[string]any{
		"campaign":    campaignToJSON(campaign),
		"lead_counts": counts,
	})
}

// Start handles POST /api/v1/campaigns/:id/start
func (h *CampaignHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	h.queries.UpdateCampaignStatus(r.Context(), repository.UpdateCampaignStatusParams{
		ID:     pgUUID(id),
		Status: pgtype.Text{String: "active", Valid: true},
	})
	h.queries.ActivateCampaignLeads(r.Context(), pgUUID(id))

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// Pause handles POST /api/v1/campaigns/:id/pause
func (h *CampaignHandler) Pause(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	h.queries.UpdateCampaignStatus(r.Context(), repository.UpdateCampaignStatusParams{
		ID:     pgUUID(id),
		Status: pgtype.Text{String: "paused", Valid: true},
	})
	h.queries.PauseCampaignLeads(r.Context(), pgUUID(id))

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// ListLeads handles GET /api/v1/campaigns/:id/leads
func (h *CampaignHandler) ListLeads(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	leads, err := h.queries.ListCampaignLeads(r.Context(), pgUUID(id))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, leads)
}
