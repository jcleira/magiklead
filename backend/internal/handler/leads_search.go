package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// LeadSearchHandler serves POST /api/v1/leads/search — the customer-
// facing query API over the canonical person/employment/organization
// graph. Plan §T12.
type LeadSearchHandler struct {
	queries *repository.Queries
}

func NewLeadSearchHandler(q *repository.Queries) *LeadSearchHandler {
	return &LeadSearchHandler{queries: q}
}

// leadSearchRequest is the wire shape for POST /api/v1/leads/search.
//
// The plan lists industries / company_size / locations alongside
// titles, but the schema doesn't carry that data yet. Keeping them
// as explicit fields here so the API rejects them rather than
// silently dropping them — once the schema lands, flip the rejection
// branches into real filters.
type leadSearchRequest struct {
	Titles      []string `json:"titles"`
	WithEmail   bool     `json:"with_email"`
	Limit       int32    `json:"limit"`
	Offset      int32    `json:"offset"`
	Industries  []string `json:"industries"`
	CompanySize string   `json:"company_size"`
	Locations   []string `json:"locations"`
}

type leadSearchResult struct {
	PersonID         string   `json:"person_id"`
	Name             string   `json:"name"`
	FirstName        *string  `json:"first_name,omitempty"`
	LastName         *string  `json:"last_name,omitempty"`
	Title            *string  `json:"title,omitempty"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationName string   `json:"organization_name"`
	Domain           *string  `json:"domain,omitempty"`
	Email            *string  `json:"email,omitempty"`
	EmailVerified    bool     `json:"email_verified"`
	EmailIsCatchall  bool     `json:"email_is_catchall"`
	TitleScore       *float32 `json:"title_score,omitempty"`
}

type leadSearchResponse struct {
	Results []leadSearchResult `json:"results"`
	Count   int                `json:"count"`
}

// Search handles POST /api/v1/leads/search.
func (h *LeadSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req leadSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}

	if len(req.Industries) > 0 || req.CompanySize != "" || len(req.Locations) > 0 {
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusNotImplemented,
			Code:    "filter_unsupported",
			Message: "industries, company_size, and locations filters require schema additions (see plan §T12)",
		})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)
	offset := max(req.Offset, 0)

	titles := normalizeTitles(req.Titles)

	rows, err := h.queries.SearchPersons(r.Context(), repository.SearchPersonsParams{
		Titles:       titles,
		WithEmail:    req.WithEmail,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
		return
	}

	emailByPerson := map[[16]byte]repository.ListBestEmailsForPersonsRow{}
	if len(rows) > 0 {
		ids := make([]pgtype.UUID, len(rows))
		for i, row := range rows {
			ids[i] = row.PersonID
		}
		emails, err := h.queries.ListBestEmailsForPersons(r.Context(), ids)
		if err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
			return
		}
		for _, e := range emails {
			emailByPerson[e.PersonID.Bytes] = e
		}
	}

	results := make([]leadSearchResult, len(rows))
	for i, row := range rows {
		results[i] = toSearchResult(row, emailByPerson[row.PersonID.Bytes])
	}

	apierr.WriteJSON(w, http.StatusOK, leadSearchResponse{
		Results: results,
		Count:   len(results),
	})
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

func toSearchResult(p repository.SearchPersonsRow, em repository.ListBestEmailsForPersonsRow) leadSearchResult {
	r := leadSearchResult{
		PersonID:         fmtUUID(p.PersonID),
		Name:             p.PersonCanonicalName,
		OrganizationID:   fmtUUID(p.OrganizationID),
		OrganizationName: p.OrganizationName,
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
	if p.PrimaryDomain.Valid {
		v := p.PrimaryDomain.String
		r.Domain = &v
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
