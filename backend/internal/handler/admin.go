package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// AdminHandler serves /api/v1/admin/*. Plan §T14.
type AdminHandler struct {
	queries *repository.Queries
	pool    *pgxpool.Pool
}

func NewAdminHandler(q *repository.Queries, pool *pgxpool.Pool) *AdminHandler {
	return &AdminHandler{queries: q, pool: pool}
}

// ----------------------------------------------------------------------------
// Conflicts
// ----------------------------------------------------------------------------

type adminConflict struct {
	ID                  string         `json:"id"`
	CanonicalTable      string         `json:"canonical_table"`
	CanonicalID         string         `json:"canonical_id"`
	NewSourceRecordID   string         `json:"new_source_record_id"`
	Status              string         `json:"status"`
	CreatedAt           string         `json:"created_at"`
	PersonCanonicalName *string        `json:"person_canonical_name,omitempty"`
	SourceName          string         `json:"source_name"`
	SourceRecordFields  map[string]any `json:"source_record_fields,omitempty"`
}

// ListConflicts handles GET /api/v1/admin/conflicts?status=pending&limit=50
func (h *AdminHandler) ListConflicts(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, offset := paginationFromQuery(r, 50, 200)

	statusArg := pgtype.Text{}
	if status != "" {
		statusArg = pgtype.Text{String: status, Valid: true}
	}

	rows, err := h.queries.ListConflicts(r.Context(), repository.ListConflictsParams{
		Status: statusArg,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "list_conflicts_failed", Message: err.Error()})
		return
	}

	out := make([]adminConflict, len(rows))
	for i, row := range rows {
		c := adminConflict{
			ID:                fmtUUID(row.ID),
			CanonicalTable:    row.CanonicalTable,
			CanonicalID:       fmtUUID(row.CanonicalID),
			NewSourceRecordID: fmtUUID(row.NewSourceRecordID),
			Status:            row.Status,
			CreatedAt:         row.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
			SourceName:        row.SourceName,
		}
		if row.PersonCanonicalName.Valid {
			v := row.PersonCanonicalName.String
			c.PersonCanonicalName = &v
		}
		if len(row.SourceRecordFields) > 0 {
			var fields map[string]any
			if err := json.Unmarshal(row.SourceRecordFields, &fields); err == nil {
				c.SourceRecordFields = fields
			}
		}
		out[i] = c
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]any{
		"results": out,
		"count":   len(out),
	})
}

// RejectConflict handles POST /api/v1/admin/conflicts/:id/reject
func (h *AdminHandler) RejectConflict(w http.ResponseWriter, r *http.Request) {
	conflictID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.queries.UpdateConflictStatus(r.Context(), repository.UpdateConflictStatusParams{
		ID:     pgUUID(conflictID),
		Status: "rejected",
	}); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "reject_failed", Message: err.Error()})
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// ----------------------------------------------------------------------------
// Person detail
// ----------------------------------------------------------------------------

type adminPersonDetail struct {
	Person          map[string]any   `json:"person"`
	Aliases         []map[string]any `json:"aliases"`
	Identifiers     []map[string]any `json:"identifiers"`
	Emails          []map[string]any `json:"emails"`
	Phones          []map[string]any `json:"phones"`
	SocialProfiles  []map[string]any `json:"social_profiles"`
	Employments     []map[string]any `json:"employments"`
	Evidence        []map[string]any `json:"evidence"`
}

// GetPerson handles GET /api/v1/admin/persons/:id
func (h *AdminHandler) GetPerson(w http.ResponseWriter, r *http.Request) {
	personID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	pgID := pgUUID(personID)
	ctx := r.Context()

	person, err := h.queries.GetPerson(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			apierr.WriteError(w, apierr.ErrNotFound)
			return
		}
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "get_person_failed", Message: err.Error()})
		return
	}

	aliases, _ := h.queries.ListPersonAliases(ctx, pgID)
	identifiers, _ := h.queries.ListPersonIdentifiers(ctx, pgID)
	emails, _ := h.queries.ListPersonEmails(ctx, pgID)
	phones, _ := h.queries.ListPersonPhones(ctx, pgID)
	socials, _ := h.queries.ListPersonSocialProfiles(ctx, pgID)
	employments, _ := h.queries.ListPersonEmployments(ctx, pgID)
	evidence, _ := h.queries.ListPersonEvidence(ctx, pgID)

	apierr.WriteJSON(w, http.StatusOK, adminPersonDetail{
		Person: map[string]any{
			"id":              fmtUUID(person.ID),
			"canonical_name":  person.CanonicalName,
			"first_name":      textOrNil(person.FirstName),
			"last_name":       textOrNil(person.LastName),
			"normalized_name": person.NormalizedName,
			"created_at":      person.CreatedAt.Time,
			"updated_at":      person.UpdatedAt.Time,
		},
		Aliases:        aliasesToJSON(aliases),
		Identifiers:    identifiersToJSON(identifiers),
		Emails:         emailsToJSON(emails),
		Phones:         phonesToJSON(phones),
		SocialProfiles: socialsToJSON(socials),
		Employments:    employmentsToJSON(employments),
		Evidence:       evidenceToJSON(evidence),
	})
}

// DeletePerson handles POST /api/v1/admin/persons/:id/delete.
//
// Hard-deletes the canonical person; CASCADE clears evidence,
// employments, emails, phones, identifiers, aliases, social profiles,
// tenant_leads. T16 will layer the deletion_blocklist write on top so
// re-ingestion can't reintroduce the same person.
func (h *AdminHandler) DeletePerson(w http.ResponseWriter, r *http.Request) {
	personID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.queries.DeletePerson(r.Context(), pgUUID(personID)); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "delete_failed", Message: err.Error()})
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ----------------------------------------------------------------------------
// Merge
// ----------------------------------------------------------------------------

type mergeRequest struct {
	// SurvivingID is the canonical person that stays; MergedID is the
	// duplicate whose evidence/employments/emails/etc. get re-pointed
	// at SurvivingID and is then hard-deleted.
	SurvivingID string `json:"surviving_id"`
	MergedID    string `json:"merged_id"`
	Reason      string `json:"reason"`
}

// MergeConflict handles POST /api/v1/admin/conflicts/:id/merge.
func (h *AdminHandler) MergeConflict(w http.ResponseWriter, r *http.Request) {
	conflictID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var req mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	survivingID, err := uuid.Parse(req.SurvivingID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid surviving_id"})
		return
	}
	mergedID, err := uuid.Parse(req.MergedID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "invalid merged_id"})
		return
	}
	if survivingID == mergedID {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "surviving_id and merged_id must differ"})
		return
	}

	adminUserID := middleware.GetAdminUserID(r.Context())

	if err := h.runMerge(r.Context(), conflictID, survivingID, mergedID, adminUserID, req.Reason); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "merge_failed", Message: err.Error()})
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{
		"status":        "merged",
		"surviving_id":  survivingID.String(),
		"merged_id":     mergedID.String(),
		"conflict_id":   conflictID.String(),
	})
}

// runMerge executes the multi-table reassignment + history record +
// deletion under one pgx.Tx. Order follows admin.sql §"Merge persons".
func (h *AdminHandler) runMerge(ctx context.Context, conflictID, survivingID, mergedID, adminUserID uuid.UUID, reason string) error {
	keep := pgUUID(survivingID)
	gone := pgUUID(mergedID)

	return pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
		q := h.queries.WithTx(tx)

		// Both persons must exist before we touch anything.
		if _, err := q.GetPerson(ctx, keep); err != nil {
			return fmt.Errorf("surviving person not found: %w", err)
		}
		if _, err := q.GetPerson(ctx, gone); err != nil {
			return fmt.Errorf("merged person not found: %w", err)
		}

		// 1. Move rows whose unique constraints might collide first.
		if err := q.MoveAliasesForMerge(ctx, repository.MoveAliasesForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("move aliases: %w", err)
		}
		if err := q.DeleteAliasesForMerged(ctx, gone); err != nil {
			return fmt.Errorf("delete merged aliases: %w", err)
		}

		if err := q.MoveIdentifiersForMerge(ctx, repository.MoveIdentifiersForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("move identifiers: %w", err)
		}
		if err := q.DeleteIdentifiersForMerged(ctx, gone); err != nil {
			return fmt.Errorf("delete merged identifiers: %w", err)
		}

		if err := q.MoveSocialProfilesForMerge(ctx, repository.MoveSocialProfilesForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("move socials: %w", err)
		}
		if err := q.DeleteSocialProfilesForMerged(ctx, gone); err != nil {
			return fmt.Errorf("delete merged socials: %w", err)
		}

		if err := q.MoveTenantLeadsForMerge(ctx, repository.MoveTenantLeadsForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("move tenant_leads: %w", err)
		}
		if err := q.DeleteTenantLeadsForMerged(ctx, gone); err != nil {
			return fmt.Errorf("delete merged tenant_leads: %w", err)
		}

		// 2. Re-point the no-conflict tables.
		if err := q.ReassignEvidenceForMerge(ctx, repository.ReassignEvidenceForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign evidence: %w", err)
		}
		if err := q.ReassignEmploymentsForMerge(ctx, repository.ReassignEmploymentsForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign employments: %w", err)
		}
		if err := q.ReassignEmailsForMerge(ctx, repository.ReassignEmailsForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign emails: %w", err)
		}
		if err := q.ReassignPhonesForMerge(ctx, repository.ReassignPhonesForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign phones: %w", err)
		}
		if err := q.ReassignCampaignLeadsForMerge(ctx, repository.ReassignCampaignLeadsForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign campaign_leads: %w", err)
		}
		if err := q.ReassignLegacyLeadsForMerge(ctx, repository.ReassignLegacyLeadsForMergeParams{KeepID: keep, MergedID: gone}); err != nil {
			return fmt.Errorf("reassign legacy leads: %w", err)
		}

		// 3. History record + delete merged person.
		var reasonArg pgtype.Text
		if reason != "" {
			reasonArg = pgtype.Text{String: reason, Valid: true}
		}
		if _, err := q.CreateMergeHistory(ctx, repository.CreateMergeHistoryParams{
			SurvivingID:    keep,
			MergedID:       gone,
			CanonicalTable: "persons",
			MergedByUserID: pgUUID(adminUserID),
			Reason:         reasonArg,
		}); err != nil {
			return fmt.Errorf("create merge_history: %w", err)
		}
		if err := q.DeletePerson(ctx, gone); err != nil {
			return fmt.Errorf("delete merged person: %w", err)
		}

		// 4. Mark the conflict resolved.
		if err := q.UpdateConflictStatus(ctx, repository.UpdateConflictStatusParams{
			ID:     pgUUID(conflictID),
			Status: "resolved",
		}); err != nil {
			return fmt.Errorf("update conflict status: %w", err)
		}
		return nil
	})
}

// ----------------------------------------------------------------------------
// JSON helpers — keep the handler functions readable.
// ----------------------------------------------------------------------------

func paginationFromQuery(r *http.Request, defaultLimit, maxLimit int32) (int32, int32) {
	limit := defaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int32
		fmt.Sscanf(v, "%d", &n)
		if n > 0 {
			limit = n
		}
	}
	limit = min(limit, maxLimit)
	offset := int32(0)
	if v := r.URL.Query().Get("offset"); v != "" {
		var n int32
		fmt.Sscanf(v, "%d", &n)
		offset = max(n, 0)
	}
	return limit, offset
}

func uuidParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func textOrNil(t pgtype.Text) any {
	if !t.Valid {
		return nil
	}
	return t.String
}

func boolOrNil(b pgtype.Bool) any {
	if !b.Valid {
		return nil
	}
	return b.Bool
}

func tsOrNil(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}

func dateOrNil(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}
	return d.Time
}

func aliasesToJSON(rows []repository.ListPersonAliasesRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":         fmtUUID(r.ID),
			"alias":      r.Alias,
			"alias_type": r.AliasType,
		}
	}
	return out
}

func identifiersToJSON(rows []repository.ListPersonIdentifiersRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":               fmtUUID(r.ID),
			"identifier_type":  r.IdentifierType,
			"identifier_value": r.IdentifierValue,
			"is_primary":       boolOrNil(r.IsPrimary),
		}
	}
	return out
}

func emailsToJSON(rows []repository.ListPersonEmailsRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":                  fmtUUID(r.ID),
			"email":               r.Email,
			"verified_at":         tsOrNil(r.VerifiedAt),
			"verification_method": textOrNil(r.VerificationMethod),
			"bounce_count":        r.BounceCount.Int32,
			"is_catchall":         boolOrNil(r.IsCatchall),
			"created_at":          tsOrNil(r.CreatedAt),
		}
	}
	return out
}

func phonesToJSON(rows []repository.ListPersonPhonesRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":          fmtUUID(r.ID),
			"phone":       r.Phone,
			"verified_at": tsOrNil(r.VerifiedAt),
			"created_at":  tsOrNil(r.CreatedAt),
		}
	}
	return out
}

func socialsToJSON(rows []repository.ListPersonSocialProfilesRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":         fmtUUID(r.ID),
			"platform":   r.Platform,
			"handle":     textOrNil(r.Handle),
			"url":        textOrNil(r.Url),
			"created_at": tsOrNil(r.CreatedAt),
		}
	}
	return out
}

func employmentsToJSON(rows []repository.ListPersonEmploymentsRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any{
			"id":                fmtUUID(r.ID),
			"title":             textOrNil(r.Title),
			"start_date":        dateOrNil(r.StartDate),
			"end_date":          dateOrNil(r.EndDate),
			"is_current":        boolOrNil(r.IsCurrent),
			"organization_id":   fmtUUID(r.OrganizationID),
			"organization_name": r.OrganizationName,
			"primary_domain":    textOrNil(r.PrimaryDomain),
			"created_at":        tsOrNil(r.CreatedAt),
		}
	}
	return out
}

func evidenceToJSON(rows []repository.ListPersonEvidenceRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, r := range rows {
		var fields map[string]any
		if len(r.Fields) > 0 {
			_ = json.Unmarshal(r.Fields, &fields)
		}
		out[i] = map[string]any{
			"id":               fmtUUID(r.ID),
			"confidence":       r.Confidence,
			"conflict_flag":    boolOrNil(r.ConflictFlag),
			"created_at":       tsOrNil(r.CreatedAt),
			"source_record_id": fmtUUID(r.SourceRecordID),
			"external_id":      textOrNil(r.ExternalID),
			"fields":           fields,
			"ingested_at":      tsOrNil(r.IngestedAt),
			"source_id":        fmtUUID(r.SourceID),
			"source_name":      r.SourceName,
			"source_type":      r.SourceType,
		}
	}
	return out
}
