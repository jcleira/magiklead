package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/leads"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

type LeadHandler struct {
	queries     *repository.Queries
	asynqClient *asynq.Client
}

func NewLeadHandler(q *repository.Queries, ac *asynq.Client) *LeadHandler {
	return &LeadHandler{queries: q, asynqClient: ac}
}

// Discover handles POST /api/v1/leads/discover
func (h *LeadHandler) Discover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PlayID     string `json:"play_id"`
		CampaignID string `json:"campaign_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	tenantID := getTenantID(r.Context())

	payload := map[string]string{
		"tenant_id":   tenantID.String(),
		"play_id":     req.PlayID,
		"campaign_id": req.CampaignID,
	}
	data, _ := json.Marshal(payload)

	task := asynq.NewTask(leads.TypeDiscoverLeads, data)
	info, err := h.asynqClient.Enqueue(task)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "queue_error", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusAccepted, map[string]string{
		"task_id": info.ID,
		"status":  "queued",
	})
}

// List handles GET /api/v1/leads
func (h *LeadHandler) List(w http.ResponseWriter, r *http.Request) {
	search := r.URL.Query().Get("q")
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if search != "" {
		result, err := h.queries.SearchLeads(r.Context(), repository.SearchLeadsParams{
			Column1: pgtype.Text{String: search, Valid: true},
			Limit:   int32(limit),
			Offset:  int32(offset),
		})
		if err != nil {
			apierr.WriteError(w, apierr.ErrInternal)
			return
		}
		apierr.WriteJSON(w, http.StatusOK, result)
		return
	}

	result, err := h.queries.ListLeads(r.Context(), repository.ListLeadsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, result)
}

// Get handles GET /api/v1/leads/:id
func (h *LeadHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	lead, err := h.queries.GetLead(r.Context(), pgUUID(id))
	if err != nil {
		apierr.WriteError(w, apierr.ErrNotFound)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, lead)
}
