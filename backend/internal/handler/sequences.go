package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/ai"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type SequenceHandler struct {
	ai      *ai.Client
	queries *repository.Queries
}

func NewSequenceHandler(aiClient *ai.Client, q *repository.Queries) *SequenceHandler {
	return &SequenceHandler{ai: aiClient, queries: q}
}

// Generate handles POST /api/v1/sequences/generate
func (h *SequenceHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayID     string `json:"play_id"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	tenantID := getTenantID(r.Context())
	if tenantID == uuid.Nil {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	if apiErr, ok := ensureSequencesQuota(r.Context(), h.queries, tenantID); !ok {
		apierr.WriteError(w, apiErr)
		return
	}

	// Get tenant profile
	tenant, err := h.queries.GetTenant(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	var profile ai.BusinessProfile
	if err := json.Unmarshal(tenant.BusinessProfile, &profile); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "no_profile", Message: "No business profile found"})
		return
	}

	// Get play
	playID, _ := uuid.Parse(req.PlayID)
	play, err := h.queries.GetPlay(r.Context(), repository.GetPlayParams{
		ID:       pgUUID(playID),
		TenantID: pgUUID(tenantID),
	})
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	// Parse ICP for the AI
	var icp ai.ICP
	json.Unmarshal(play.Icp, &icp)

	aiPlay := &ai.Play{
		Name:     play.Name,
		ICP:      icp,
		Channels: play.Channels,
	}

	// Generate sequence with a sample lead
	sampleLead := &ai.LeadInfo{
		FirstName: "{{first_name}}",
		LastName:  "{{last_name}}",
		Title:     "{{title}}",
		Company:   "{{company}}",
	}

	seq, err := h.ai.GenerateSequence(r.Context(), &profile, aiPlay, sampleLead)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "ai_error", Message: err.Error()})
		return
	}

	// Store sequence on campaign
	campaignID, _ := uuid.Parse(req.CampaignID)
	seqJSON, _ := json.Marshal(seq.Steps)
	h.queries.UpdateCampaignSequence(r.Context(), repository.UpdateCampaignSequenceParams{
		ID:       pgUUID(campaignID),
		Sequence: seqJSON,
	})

	if err := h.queries.IncrementSequencesUsed(r.Context(), repository.IncrementSequencesUsedParams{
		TenantID:      pgUUID(tenantID),
		SequencesUsed: pgtype.Int4{Int32: 1, Valid: true},
	}); err != nil {
		// Log-and-continue for counter drift; see tenant_leads.Add.
		_ = err
	}

	apierr.WriteJSON(w, http.StatusOK, seq)
}
