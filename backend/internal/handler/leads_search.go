package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/leads/pdl"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// LeadSearchHandler serves POST /api/v1/leads/search. The canonical
// person graph is consulted first; on a miss (no fresh matches AND
// at least one PDL-only filter active) PDL is called synchronously,
// results write through to the canonical, and the canonical is
// re-queried. Issue #7.
type LeadSearchHandler struct {
	queries *repository.Queries
	pdl     *pdl.Module
}

// NewLeadSearchHandler constructs the handler. Pass nil for `pdlModule`
// to disable the PDL fallback (dev-mode without PDL_API_KEY); the
// handler then behaves as canonical-only.
func NewLeadSearchHandler(q *repository.Queries, pdlModule *pdl.Module) *LeadSearchHandler {
	return &LeadSearchHandler{queries: q, pdl: pdlModule}
}

// leadSearchRequest is the wire shape for POST /api/v1/leads/search.
// The PDL-specific filters land alongside the existing titles
// filter; `description` carries the free-text ICP from the
// onboarding flow.
type leadSearchRequest struct {
	Titles      []string `json:"titles"`
	WithEmail   bool     `json:"with_email"`
	Limit       int32    `json:"limit"`
	Offset      int32    `json:"offset"`
	Industries  []string `json:"industries"`
	CompanySize string   `json:"company_size"`
	Locations   []string `json:"locations"`
	Description string   `json:"description"`
}

type leadSearchResult struct {
	PersonID         string   `json:"person_id"`
	Name             string   `json:"name"`
	FirstName        *string  `json:"first_name,omitempty"`
	LastName         *string  `json:"last_name,omitempty"`
	Title            *string  `json:"title,omitempty"`
	Location         *string  `json:"location,omitempty"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationName string   `json:"organization_name"`
	Domain           *string  `json:"domain,omitempty"`
	Industries       []string `json:"industries,omitempty"`
	CompanySize      *string  `json:"company_size,omitempty"`
	Email            *string  `json:"email,omitempty"`
	EmailVerified    bool     `json:"email_verified"`
	EmailIsCatchall  bool     `json:"email_is_catchall"`
	TitleScore       *float32 `json:"title_score,omitempty"`
}

type leadSearchResponse struct {
	Results   []leadSearchResult `json:"results"`
	Count     int                `json:"count"`
	PDLCalled bool               `json:"pdl_called"`
}

// Search handles POST /api/v1/leads/search.
func (h *LeadSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req leadSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	if apiErr, ok := ensureLeadsQuota(r.Context(), h.queries, getTenantID(r.Context())); !ok {
		apierr.WriteError(w, apiErr)
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	offset := max(req.Offset, 0)

	titles := normalizeTitles(req.Titles)
	industries := normalizeTitles(req.Industries)
	locations := normalizeTitles(req.Locations)
	companySize := strings.TrimSpace(req.CompanySize)
	description := strings.TrimSpace(req.Description)

	usesPDLFilters := len(industries) > 0 || companySize != "" || len(locations) > 0 || description != ""
	pdlCalled := false

	// Canonical-first query. When any PDL-only filter is active we
	// require fresh rows so stale matches don't masquerade as a hit.
	rows, err := h.searchCanonical(r.Context(), titles, industries, companySize, locations,
		req.WithEmail, usesPDLFilters, limit, offset)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
		return
	}

	if len(rows) == 0 && usesPDLFilters && h.pdl != nil && h.pdl.Configured() {
		if _, err := h.pdl.Search(r.Context(), pdl.Filters{
			Titles:      titles,
			Industries:  industries,
			CompanySize: companySize,
			Locations:   locations,
			Description: description,
			Limit:       int(limit),
		}); err != nil {
			if !isRetryableLookupError(err) {
				apierr.WriteError(w, apierr.APIError{Status: 502, Code: "pdl_failed", Message: err.Error()})
				return
			}
			log.Printf("pdl: search soft-failed: %v", err)
		}
		pdlCalled = true
		rows, err = h.searchCanonical(r.Context(), titles, industries, companySize, locations,
			req.WithEmail, usesPDLFilters, limit, offset)
		if err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
			return
		}
	}

	emailByPerson, err := h.attachEmails(r.Context(), rows)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
		return
	}
	results := make([]leadSearchResult, len(rows))
	for i, row := range rows {
		results[i] = toSearchResult(row, emailByPerson[row.PersonID.Bytes])
	}

	apierr.WriteJSON(w, http.StatusOK, leadSearchResponse{
		Results:   results,
		Count:     len(results),
		PDLCalled: pdlCalled,
	})
}

func (h *LeadSearchHandler) searchCanonical(ctx context.Context, titles, industries []string, companySize string,
	locations []string, withEmail, requireFresh bool, limit, offset int32) ([]repository.SearchPersonsRow, error) {
	return h.queries.SearchPersons(ctx, repository.SearchPersonsParams{
		Titles:       titles,
		WithEmail:    withEmail,
		Industries:   lowerEach(industries),
		CompanySize:  pgTextOrNull(companySize),
		Locations:    locations,
		RequireFresh: requireFresh,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
}

func (h *LeadSearchHandler) attachEmails(ctx context.Context, rows []repository.SearchPersonsRow) (map[[16]byte]repository.ListBestEmailsForPersonsRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.PersonID
	}
	emails, err := h.queries.ListBestEmailsForPersons(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[[16]byte]repository.ListBestEmailsForPersonsRow, len(emails))
	for _, e := range emails {
		out[e.PersonID.Bytes] = e
	}
	return out, nil
}

// isRetryableLookupError returns true for PDL conditions where it's
// acceptable to keep serving the canonical (possibly empty) response
// without surfacing a 502: missing key, no results, etc. Hard errors
// (auth failure, credit exhausted, rate limit) still fail the request
// so the operator notices in dev.
func isRetryableLookupError(err error) bool {
	return errors.Is(err, pdl.ErrNotConfigured)
}

// normalizeTitles trims and drops empties; returns nil when no
// usable titles remain so the SQL treats it as "no title filter".
func normalizeTitles(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lowerEach(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

func pgTextOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func toSearchResult(p repository.SearchPersonsRow, em repository.ListBestEmailsForPersonsRow) leadSearchResult {
	r := leadSearchResult{
		PersonID:         fmtUUID(p.PersonID),
		Name:             p.PersonCanonicalName,
		OrganizationID:   fmtUUID(p.OrganizationID),
		OrganizationName: p.OrganizationName,
		Industries:       p.Industries,
	}
	if p.FirstName.Valid {
		v := p.FirstName.String
		r.FirstName = &v
	}
	if p.LastName.Valid {
		v := p.LastName.String
		r.LastName = &v
	}
	if p.Title.Valid {
		v := p.Title.String
		r.Title = &v
	}
	if p.Location.Valid {
		v := p.Location.String
		r.Location = &v
	}
	if p.PrimaryDomain.Valid {
		v := p.PrimaryDomain.String
		r.Domain = &v
	}
	if p.SizeRange.Valid {
		v := p.SizeRange.String
		r.CompanySize = &v
	}
	if p.TitleScore > 0 {
		v := p.TitleScore
		r.TitleScore = &v
	}
	if em.Email != "" {
		r.Email = &em.Email
		r.EmailVerified = em.VerifiedAt.Valid
		r.EmailIsCatchall = em.IsCatchall.Valid && em.IsCatchall.Bool
	}
	return r
}
