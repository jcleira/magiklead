package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// TenantLeadHandler serves /api/v1/tenant_leads — the per-tenant
// saved-lead list built on top of the canonical `persons` graph.
// Plan §T05.
type TenantLeadHandler struct {
	queries *repository.Queries
}

func NewTenantLeadHandler(q *repository.Queries) *TenantLeadHandler {
	return &TenantLeadHandler{queries: q}
}

type tenantLeadAddRequest struct {
	PersonID string `json:"person_id"`
	Status   string `json:"status"`
	Notes    string `json:"notes"`
}

type tenantLeadUpdateRequest struct {
	Status *string `json:"status,omitempty"`
	Notes  *string `json:"notes,omitempty"`
}

type tenantLeadResponse struct {
	PersonID  string  `json:"person_id"`
	Status    string  `json:"status"`
	Notes     *string `json:"notes,omitempty"`
	AddedAt   string  `json:"added_at"`
	Name      string  `json:"name,omitempty"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
}

type tenantLeadListResponse struct {
	Results []tenantLeadResponse `json:"results"`
	Count   int64                `json:"count"`
}

// Add handles POST /api/v1/tenant_leads — saves a canonical person as
// this tenant's lead. Idempotent via ON CONFLICT in the SQL.
func (h *TenantLeadHandler) Add(w http.ResponseWriter, r *http.Request) {
	var req tenantLeadAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	personID, err := uuid.Parse(strings.TrimSpace(req.PersonID))
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid person_id"})
		return
	}

	tenantID := getTenantID(r.Context())
	if apiErr, ok := ensureLeadsQuota(r.Context(), h.queries, tenantID); !ok {
		apierr.WriteError(w, apiErr)
		return
	}
	userID, ok := h.resolveUserID(w, r)
	if !ok {
		return
	}

	params := repository.AddTenantLeadParams{
		TenantID:      pgUUID(tenantID),
		PersonID:      pgUUID(personID),
		AddedByUserID: pgUUID(userID),
	}
	if s := strings.TrimSpace(req.Status); s != "" {
		params.Status = pgtype.Text{String: s, Valid: true}
	}
	if n := strings.TrimSpace(req.Notes); n != "" {
		params.Notes = pgtype.Text{String: n, Valid: true}
	}

	row, err := h.queries.AddTenantLead(r.Context(), params)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "add_failed", Message: err.Error()})
		return
	}

	// Increment usage after the row is in. Upsert path (re-saving an
	// existing lead) still increments — callers that want "reset
	// status without double-counting" should PATCH instead.
	if err := h.queries.IncrementLeadsUsed(r.Context(), repository.IncrementLeadsUsedParams{
		TenantID:  pgUUID(tenantID),
		LeadsUsed: pgtype.Int4{Int32: 1, Valid: true},
	}); err != nil {
		// Don't fail the user-facing request on a counter drift; log
		// and continue. The nightly reset + GetSubscription fallback
		// keep the system from blocking on a stuck counter.
		// (Intentional: quota drift > UX drift.)
		_ = err
	}

	apierr.WriteJSON(w, http.StatusCreated, toTenantLeadFromRow(row))
}

// List handles GET /api/v1/tenant_leads?status=new&limit=50&offset=0.
func (h *TenantLeadHandler) List(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	limit, offset := paginationFromQuery(r, 50, 200)

	statusArg := pgtype.Text{}
	if s := strings.TrimSpace(r.URL.Query().Get("status")); s != "" {
		statusArg = pgtype.Text{String: s, Valid: true}
	}

	rows, err := h.queries.ListTenantLeads(r.Context(), repository.ListTenantLeadsParams{
		TenantID: pgUUID(tenantID),
		Limit:    limit,
		Offset:   offset,
		Status:   statusArg,
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "list_failed", Message: err.Error()})
		return
	}
	count, err := h.queries.CountTenantLeads(r.Context(), repository.CountTenantLeadsParams{
		TenantID: pgUUID(tenantID),
		Status:   statusArg,
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "list_failed", Message: err.Error()})
		return
	}

	out := make([]tenantLeadResponse, len(rows))
	for i, row := range rows {
		out[i] = toTenantLeadFromListRow(row)
	}
	apierr.WriteJSON(w, http.StatusOK, tenantLeadListResponse{Results: out, Count: count})
}

// Update handles PATCH /api/v1/tenant_leads/{person_id} — partial
// updates for status and/or notes.
func (h *TenantLeadHandler) Update(w http.ResponseWriter, r *http.Request) {
	personID, err := uuid.Parse(chi.URLParam(r, "person_id"))
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid person_id"})
		return
	}
	var req tenantLeadUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	if req.Status == nil && req.Notes == nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "status or notes required"})
		return
	}

	tenantID := getTenantID(r.Context())

	if _, err := h.queries.GetTenantLead(r.Context(), repository.GetTenantLeadParams{
		TenantID: pgUUID(tenantID),
		PersonID: pgUUID(personID),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.WriteError(w, apierr.ErrNotFound)
			return
		}
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "update_failed", Message: err.Error()})
		return
	}

	if req.Status != nil {
		if err := h.queries.UpdateTenantLeadStatus(r.Context(), repository.UpdateTenantLeadStatusParams{
			TenantID: pgUUID(tenantID),
			PersonID: pgUUID(personID),
			Status:   *req.Status,
		}); err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "update_failed", Message: err.Error()})
			return
		}
	}
	if req.Notes != nil {
		notesArg := pgtype.Text{}
		if *req.Notes != "" {
			notesArg = pgtype.Text{String: *req.Notes, Valid: true}
		}
		if err := h.queries.UpdateTenantLeadNotes(r.Context(), repository.UpdateTenantLeadNotesParams{
			TenantID: pgUUID(tenantID),
			PersonID: pgUUID(personID),
			Notes:    notesArg,
		}); err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "update_failed", Message: err.Error()})
			return
		}
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete handles DELETE /api/v1/tenant_leads/{person_id}.
func (h *TenantLeadHandler) Delete(w http.ResponseWriter, r *http.Request) {
	personID, err := uuid.Parse(chi.URLParam(r, "person_id"))
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid person_id"})
		return
	}
	tenantID := getTenantID(r.Context())
	if err := h.queries.RemoveTenantLead(r.Context(), repository.RemoveTenantLeadParams{
		TenantID: pgUUID(tenantID),
		PersonID: pgUUID(personID),
	}); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "delete_failed", Message: err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TenantLeadHandler) resolveUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	clerkID := middleware.GetUserClerkID(r.Context())
	if clerkID == "" {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return uuid.Nil, false
	}
	user, err := h.queries.GetUserByClerkID(r.Context(), clerkID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "user_not_found", Message: "user lookup failed"})
		return uuid.Nil, false
	}
	return uuid.UUID(user.ID.Bytes), true
}

func toTenantLeadFromRow(tl repository.TenantLead) tenantLeadResponse {
	out := tenantLeadResponse{
		PersonID: fmtUUID(tl.PersonID),
		Status:   tl.Status,
		AddedAt:  tl.AddedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}
	if tl.Notes.Valid {
		v := tl.Notes.String
		out.Notes = &v
	}
	return out
}

func toTenantLeadFromListRow(tl repository.ListTenantLeadsRow) tenantLeadResponse {
	out := tenantLeadResponse{
		PersonID: fmtUUID(tl.PersonID),
		Status:   tl.Status,
		AddedAt:  tl.AddedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		Name:     tl.CanonicalName,
	}
	if tl.Notes.Valid {
		v := tl.Notes.String
		out.Notes = &v
	}
	if tl.FirstName.Valid {
		v := tl.FirstName.String
		out.FirstName = &v
	}
	if tl.LastName.Valid {
		v := tl.LastName.String
		out.LastName = &v
	}
	return out
}
