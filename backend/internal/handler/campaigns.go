package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type CampaignHandler struct {
	queries *repository.Queries
	supp    *suppression.Module
}

// NewCampaignHandler wires the campaign routes. supp is used by the
// re-engage path (issue #4) to clear a reply-suppression and
// reactivate a lead; the rest of the routes don't need it. The
// existing two-arg call sites stay compiling by passing nil for supp
// — the Reengage route panics-with-503 in that case rather than
// silently no-op'ing.
func NewCampaignHandler(q *repository.Queries, supp *suppression.Module) *CampaignHandler {
	return &CampaignHandler{queries: q, supp: supp}
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

// AddLeads handles POST /api/v1/campaigns/:id/leads — seeds a campaign
// from the tenant's saved-leads list. Each person_id must already
// exist in tenant_leads for this tenant; otherwise it's silently
// skipped (the response reports how many landed). Plan §T06.
func (h *CampaignHandler) AddLeads(w http.ResponseWriter, r *http.Request) {
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

	var req struct {
		PersonIDs []string `json:"person_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	if len(req.PersonIDs) == 0 {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "person_ids required"})
		return
	}

	added := 0
	skipped := 0
	for _, raw := range req.PersonIDs {
		personID, err := uuid.Parse(raw)
		if err != nil {
			skipped++
			continue
		}
		if _, err := h.queries.GetTenantLead(r.Context(), repository.GetTenantLeadParams{
			TenantID: pgUUID(tenantID),
			PersonID: pgUUID(personID),
		}); err != nil {
			skipped++
			continue
		}
		if err := h.queries.AddPersonToCampaign(r.Context(), repository.AddPersonToCampaignParams{
			CampaignID: pgUUID(uuid.UUID(campaign.ID.Bytes)),
			PersonID:   pgUUID(personID),
		}); err != nil {
			skipped++
			continue
		}
		added++
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]int{"added": added, "skipped": skipped})
}

// Reengage handles POST /api/v1/campaigns/{id}/leads/{lead-id}/reengage.
// It's the operator's escape hatch for "they replied with an
// out-of-office, not a real reply" (user story 26). The handler
// validates the URL params, confirms the lead belongs to a campaign
// in the caller's tenant, and asks the suppression module to clear
// the reply gate + reactivate the lead. Only the reply reason is
// reversible; other suppression reasons stay in place.
func (h *CampaignHandler) Reengage(w http.ResponseWriter, r *http.Request) {
	if h.supp == nil {
		apierr.WriteError(w, apierr.APIError{Status: 503, Code: "suppression_unavailable", Message: "suppression module not wired"})
		return
	}
	campaignID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid campaign id"})
		return
	}
	leadID, err := uuid.Parse(chi.URLParam(r, "lead_id"))
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid lead id"})
		return
	}

	tenantID := getTenantID(r.Context())
	// Confirm the lead belongs to a campaign in this tenant. Without
	// this, any tenant could re-engage another tenant's leads by
	// guessing UUIDs.
	addr, err := h.queries.ResolveCampaignLeadAddress(r.Context(), pgUUID(leadID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}
	if !addr.TenantID.Valid || uuid.UUID(addr.TenantID.Bytes) != tenantID {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	if err := h.supp.ClearReply(r.Context(), leadID); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "reengage_failed", Message: err.Error()})
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]any{
		"status":      "active",
		"campaign_id": campaignID.String(),
		"lead_id":     leadID.String(),
	})
}

// Metrics handles GET /api/v1/campaigns/{id}/metrics — the data behind
// the per-campaign dashboard (issue #9). Returns five aggregations
// derived from `campaign_leads`, `email_events`, and `unsubscribes`:
// total leads, total sent (with per-step breakdown), total replied,
// total bounced (one address with multiple DSNs counts as one), and
// total unsubscribed.
//
// Tenant scope is enforced via GetCampaign, mirroring the rest of the
// campaign routes: a 404 for a campaign that exists under another
// tenant is the same response shape as a campaign that does not exist
// at all, so the endpoint never confirms cross-tenant IDs.
func (h *CampaignHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())
	if _, err := h.queries.GetCampaign(r.Context(), repository.GetCampaignParams{ID: pgUUID(id), TenantID: pgUUID(tenantID)}); err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}

	ctx := r.Context()
	campaignID := pgUUID(id)

	leadsTotal, err := h.queries.CountCampaignLeadsTotal(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}
	sentTotal, err := h.queries.CountCampaignSentTotal(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}
	sentByStepRows, err := h.queries.CountCampaignSentByStep(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}
	repliedTotal, err := h.queries.CountCampaignRepliedTotal(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}
	bouncedTotal, err := h.queries.CountCampaignBouncedTotal(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}
	unsubscribedTotal, err := h.queries.CountCampaignUnsubscribedTotal(ctx, campaignID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "metrics_failed", Message: err.Error()})
		return
	}

	// Re-shape the per-step rows into the documented JSON contract.
	// Empty steps stay absent rather than turning into zero-count rows
	// — the frontend treats missing steps as "nothing sent yet."
	sentByStep := make([]map[string]any, 0, len(sentByStepRows))
	for _, row := range sentByStepRows {
		sentByStep = append(sentByStep, map[string]any{
			"step_order": row.StepOrder,
			"count":      row.Count,
		})
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]any{
		"leads_total":        leadsTotal,
		"sent_total":         sentTotal,
		"sent_by_step":       sentByStep,
		"replied_total":      repliedTotal,
		"bounced_total":      bouncedTotal,
		"unsubscribed_total": unsubscribedTotal,
	})
}
