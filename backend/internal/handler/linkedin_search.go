package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	linkedinsearch "github.com/jcleira/magiklead/backend/internal/leads/linkedin_search"
	"github.com/jcleira/magiklead/backend/internal/linkedin/liurl"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// linkedInSearchDailyCap is each connected account's budget of search
// profiles in a rolling 24 hours. The search runs AS the account, and
// LinkedIn watches the volume: Unipile's guidance is at most 1,000
// profiles a day on a standard account. The founder set 250 on
// 2026-09-25.
const linkedInSearchDailyCap = 250

// linkedInSearchPageSize is the number of people one search call asks
// for. 10 is the people-page maximum Unipile documents.
const linkedInSearchPageSize = 10

// searchParamTTL is how long a LinkedIn id lookup stays cached. Each
// lookup runs as the account (LinkedIn's filter typeahead); the ids do
// not change.
const searchParamTTL = 24 * time.Hour

// LinkedInSearchHandler serves POST /api/v1/leads/linkedin-search — a
// Sales Navigator people search run as the tenant's connected LinkedIn
// account, through Unipile. It is the LinkedIn rail's lead source. Each
// result with a public profile writes through to the canonical graph,
// keyed on its LinkedIn URL, so the user saves it like any search
// result. A result that LinkedIn hides (no public profile) is counted
// and dropped: the rail cannot invite it.
type LinkedInSearchHandler struct {
	queries  *repository.Queries
	unipile  *unipile.Module
	writer   *linkedinsearch.Module
	params   *searchParamCache
	dailyCap int
}

// NewLinkedInSearchHandler wires the search. u is the Unipile module (the
// handler answers 503 when it is not configured) and w the canonical
// write-through.
func NewLinkedInSearchHandler(q *repository.Queries, u *unipile.Module, w *linkedinsearch.Module) *LinkedInSearchHandler {
	return &LinkedInSearchHandler{
		queries:  q,
		unipile:  u,
		writer:   w,
		params:   &searchParamCache{entries: map[string]searchParamEntry{}},
		dailyCap: linkedInSearchDailyCap,
	}
}

type linkedInSearchRequest struct {
	Titles      []string `json:"titles"`
	Locations   []string `json:"locations"`
	Industries  []string `json:"industries"`
	CompanySize string   `json:"company_size"`
	// Cursor continues an earlier search; send the same filters with it.
	Cursor string `json:"cursor"`
}

type linkedInSearchResult struct {
	PersonID          string `json:"person_id"`
	Name              string `json:"name"`
	FirstName         string `json:"first_name,omitempty"`
	LastName          string `json:"last_name,omitempty"`
	Title             string `json:"title,omitempty"`
	Company           string `json:"company,omitempty"`
	Location          string `json:"location,omitempty"`
	Headline          string `json:"headline,omitempty"`
	LinkedInURL       string `json:"linkedin_url"`
	PendingInvitation bool   `json:"pending_invitation"`
}

// matchedParam shows which LinkedIn value a typed filter matched, so the
// UI can say "United States → United States".
type matchedParam struct {
	Query string `json:"query"`
	ID    string `json:"id"`
	Title string `json:"title"`
}

type linkedInSearchResponse struct {
	Results []linkedInSearchResult `json:"results"`
	Count   int                    `json:"count"`
	// Hidden counts results LinkedIn returned without a public profile.
	Hidden int `json:"hidden"`
	// Cursor fetches the next page; "" when there is none.
	Cursor string `json:"cursor"`
	// Total is LinkedIn's result count when it gives one, else 0.
	Total      int            `json:"total"`
	Locations  []matchedParam `json:"locations"`
	Industries []matchedParam `json:"industries"`
	UsedToday  int64          `json:"used_today"`
	DailyCap   int            `json:"daily_cap"`
}

// Search handles POST /api/v1/leads/linkedin-search.
func (h *LinkedInSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	var req linkedInSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	if !h.unipile.Configured() {
		apierr.WriteError(w, apierr.APIError{Status: http.StatusServiceUnavailable, Code: "linkedin_disabled", Message: "LinkedIn integration is not configured"})
		return
	}

	titles := normalizeTitles(req.Titles)
	locations := normalizeTitles(req.Locations)
	industries := normalizeTitles(req.Industries)
	companySize := strings.TrimSpace(req.CompanySize)
	if len(titles) == 0 && len(locations) == 0 && len(industries) == 0 && companySize == "" {
		apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "Give at least one title, location, industry or company size"})
		return
	}
	var headcount *unipile.Headcount
	if companySize != "" {
		hc, ok := unipile.HeadcountForSize(companySize)
		if !ok {
			apierr.WriteError(w, apierr.APIError{Status: 400, Code: "bad_request", Message: "unknown company_size " + companySize})
			return
		}
		headcount = &hc
	}

	ctx := r.Context()
	tenantID := getTenantID(ctx)
	account, ok, err := h.searchAccount(ctx, pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	if !ok {
		apierr.WriteError(w, apierr.APIError{Status: http.StatusConflict, Code: "linkedin_not_connected", Message: "Connect a LinkedIn account in Settings first"})
		return
	}

	used, err := h.queries.SumLinkedInSearchProfilesSince(ctx, repository.SumLinkedInSearchProfilesSinceParams{
		AccountID: account.ID,
		Since:     pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	remaining := int64(h.dailyCap) - used
	if remaining <= 0 {
		apierr.WriteError(w, apierr.APIError{Status: http.StatusTooManyRequests, Code: "linkedin_search_limit",
			Message: "This LinkedIn account used its search budget for the last 24 hours. Try again later."})
		return
	}

	matchedLocations, apiErr := h.resolveParams(ctx, account, unipile.ParamRegion, locations, "unknown_location", "LinkedIn has no location that matches ")
	if apiErr != nil {
		apierr.WriteError(w, *apiErr)
		return
	}
	matchedIndustries, apiErr := h.resolveParams(ctx, account, unipile.ParamIndustry, industries, "unknown_industry", "LinkedIn has no industry that matches ")
	if apiErr != nil {
		apierr.WriteError(w, *apiErr)
		return
	}

	page, err := h.unipile.SearchPeople(ctx, account.UnipileAccountID, unipile.PeopleSearch{
		Titles:      titles,
		RegionIDs:   paramIDs(matchedLocations),
		IndustryIDs: paramIDs(matchedIndustries),
		Headcount:   headcount,
		Cursor:      strings.TrimSpace(req.Cursor),
		Limit:       int(min(remaining, linkedInSearchPageSize)),
	})
	if err != nil {
		apierr.WriteError(w, h.searchError(ctx, account, err))
		return
	}

	filters, _ := json.Marshal(map[string]any{
		"titles":       titles,
		"locations":    matchedLocations,
		"industries":   matchedIndustries,
		"company_size": companySize,
		"next_page":    req.Cursor != "",
	})
	if err := h.queries.CreateLinkedInSearch(ctx, repository.CreateLinkedInSearchParams{
		TenantID:          pgUUID(tenantID),
		LinkedinAccountID: account.ID,
		Filters:           filters,
		ResultCount:       int32(len(page.People)),
	}); err != nil {
		// The search already ran; answer with its results. A failed log
		// only means this page does not count against the budget.
		log.Printf("linkedin search: log search for account %s: %v", fmtUUID(account.ID), err)
	}

	resp := linkedInSearchResponse{
		Results:    []linkedInSearchResult{},
		Cursor:     page.Cursor,
		Total:      page.Total,
		Locations:  matchedLocations,
		Industries: matchedIndustries,
		UsedToday:  used + int64(len(page.People)),
		DailyCap:   h.dailyCap,
	}
	byURL := map[string]unipile.SearchPerson{}
	profiles := make([]linkedinsearch.Profile, 0, len(page.People))
	for _, p := range page.People {
		url := liurl.Canonical(p.ProfileURL)
		if url == "" {
			resp.Hidden++
			continue
		}
		byURL[url] = p
		profiles = append(profiles, linkedinsearch.Profile{
			FullName:          p.Name,
			FirstName:         p.FirstName,
			LastName:          p.LastName,
			Title:             personTitle(p),
			CompanyName:       p.Company,
			CompanyLinkedInID: p.CompanyID,
			LinkedInURL:       url,
			Location:          p.Location,
		})
	}
	people, err := h.writer.WriteThrough(ctx, profiles)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "search_failed", Message: err.Error()})
		return
	}
	for _, person := range people {
		src := byURL[person.LinkedInURL]
		resp.Results = append(resp.Results, linkedInSearchResult{
			PersonID:          person.PersonID.String(),
			Name:              person.Name,
			FirstName:         person.FirstName,
			LastName:          person.LastName,
			Title:             person.Title,
			Company:           person.OrganizationName,
			Location:          person.Location,
			Headline:          src.Headline,
			LinkedInURL:       person.LinkedInURL,
			PendingInvitation: src.PendingInvitation,
		})
	}
	resp.Count = len(resp.Results)
	apierr.WriteJSON(w, http.StatusOK, resp)
}

// searchAccount picks the tenant's connected account that can act —
// active or warming, as the invite worker picks it.
func (h *LinkedInSearchHandler) searchAccount(ctx context.Context, tenantID pgtype.UUID) (repository.LinkedinAccount, bool, error) {
	accounts, err := h.queries.ListLinkedInAccounts(ctx, tenantID)
	if err != nil {
		return repository.LinkedinAccount{}, false, err
	}
	for _, a := range accounts {
		if a.Status == "active" || a.Status == "warming" {
			return a, true, nil
		}
	}
	return repository.LinkedinAccount{}, false, nil
}

// resolveParams turns each typed value into a LinkedIn id. A value
// LinkedIn does not know is a 400 that names it.
func (h *LinkedInSearchHandler) resolveParams(ctx context.Context, account repository.LinkedinAccount, paramType string, values []string, code, message string) ([]matchedParam, *apierr.APIError) {
	out := make([]matchedParam, 0, len(values))
	for _, v := range values {
		param, found, err := h.resolveParam(ctx, account.UnipileAccountID, paramType, v)
		if err != nil {
			apiErr := h.searchError(ctx, account, err)
			return nil, &apiErr
		}
		if !found {
			return nil, &apierr.APIError{Status: 400, Code: code, Message: message + `"` + v + `"`}
		}
		out = append(out, matchedParam{Query: v, ID: param.ID, Title: param.Title})
	}
	return out, nil
}

// resolveParam looks one value up, from the cache when it can. LinkedIn
// ranks the hits; an exact name match wins over the first hit.
func (h *LinkedInSearchHandler) resolveParam(ctx context.Context, unipileAccountID, paramType, value string) (unipile.SearchParameter, bool, error) {
	key := paramType + "\x00" + strings.ToLower(value)
	if e, ok := h.params.get(key); ok {
		return e.param, e.found, nil
	}
	hits, err := h.unipile.SearchParameters(ctx, unipileAccountID, paramType, value, 5)
	if err != nil {
		return unipile.SearchParameter{}, false, err
	}
	e := searchParamEntry{found: len(hits) > 0}
	if e.found {
		e.param = hits[0]
		for _, hit := range hits {
			if strings.EqualFold(hit.Title, value) {
				e.param = hit
				break
			}
		}
	}
	h.params.put(key, e)
	return e.param, e.found, nil
}

// searchError maps a Unipile error to the response. A restriction also
// marks the account restricted, as a refused send does, so the worker
// and Settings see it at once.
func (h *LinkedInSearchHandler) searchError(ctx context.Context, account repository.LinkedinAccount, err error) apierr.APIError {
	switch {
	case errors.Is(err, unipile.ErrAccountRestricted):
		if serr := h.queries.SetLinkedInAccountStatusByUnipileID(ctx, repository.SetLinkedInAccountStatusByUnipileIDParams{
			UnipileAccountID: account.UnipileAccountID,
			Status:           "restricted",
			LastError:        pgtype.Text{String: err.Error(), Valid: true},
		}); serr != nil {
			log.Printf("linkedin search: mark account %s restricted: %v", account.UnipileAccountID, serr)
		}
		return apierr.APIError{Status: http.StatusConflict, Code: "linkedin_restricted", Message: "LinkedIn restricted the connected account. Reconnect it in Settings."}
	case errors.Is(err, unipile.ErrRateLimited):
		return apierr.APIError{Status: http.StatusTooManyRequests, Code: "linkedin_rate_limited", Message: "LinkedIn asked to slow down. Try again later."}
	default:
		log.Printf("linkedin search: account %s: %v", account.UnipileAccountID, err)
		msg := "LinkedIn search failed"
		var se *unipile.SearchError
		if errors.As(err, &se) && se.Detail != "" {
			msg += ": " + se.Detail
		}
		return apierr.APIError{Status: http.StatusBadGateway, Code: "linkedin_search_failed", Message: msg}
	}
}

// personTitle is the current role, or the headline when LinkedIn gives
// no current position.
func personTitle(p unipile.SearchPerson) string {
	if p.Title != "" {
		return p.Title
	}
	return p.Headline
}

func paramIDs(params []matchedParam) []string {
	ids := make([]string, len(params))
	for i, p := range params {
		ids[i] = p.ID
	}
	return ids
}

type searchParamEntry struct {
	param   unipile.SearchParameter
	found   bool
	expires time.Time
}

// searchParamCache keeps LinkedIn id lookups for searchParamTTL.
type searchParamCache struct {
	mu      sync.Mutex
	entries map[string]searchParamEntry
}

func (c *searchParamCache) get(key string) (searchParamEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		return searchParamEntry{}, false
	}
	return e, true
}

func (c *searchParamCache) put(key string, e searchParamEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.expires = time.Now().Add(searchParamTTL)
	c.entries[key] = e
}
