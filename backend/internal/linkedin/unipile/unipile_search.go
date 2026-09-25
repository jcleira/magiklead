package unipile

// LinkedIn people search through Unipile — the lead source of the
// LinkedIn rail. The search runs AS the connected LinkedIn account, on
// the Sales Navigator API. A classic search is no use for prospecting
// from a small or free account: LinkedIn returns everyone outside the
// account's network as "LinkedIn Member", with no name and no profile
// slug (live capture, 2026-09-25, docs/2026-07-13-magikshot-linkedin-smoke).
// Sales Navigator shows those names and adds company-size filters.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Search-parameter types for SearchParameters: the id vocabularies of a
// Sales Navigator people search.
const (
	ParamRegion   = "REGION"
	ParamIndustry = "SALES_INDUSTRY"
)

// SearchParameter is one hit of a search-parameter lookup: the LinkedIn
// id to put in a search, and the name LinkedIn shows for it.
type SearchParameter struct {
	ID    string
	Title string
}

type searchParametersResponse struct {
	Items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"items"`
}

// SearchParameters turns free text (a region, an industry) into LinkedIn
// search ids, best match first:
//
//	GET /api/v1/linkedin/search/parameters?account_id=<acc>&type=<type>&keywords=<text>&limit=<n>
//
// The lookup runs as the account, like LinkedIn's own filter typeahead.
func (m *Module) SearchParameters(ctx context.Context, accountID, paramType, keywords string, limit int) ([]SearchParameter, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("account_id", accountID)
	q.Set("type", paramType)
	q.Set("keywords", keywords)
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	raw, err := m.searchCall(ctx, http.MethodGet, "/api/v1/linkedin/search/parameters?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var r searchParametersResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	out := make([]SearchParameter, 0, len(r.Items))
	for _, it := range r.Items {
		if it.ID != "" {
			out = append(out, SearchParameter{ID: it.ID, Title: it.Title})
		}
	}
	return out, nil
}

// Headcount is a Sales Navigator company-size bucket. Max 0 means no
// upper bound.
type Headcount struct {
	Min int
	Max int
}

// HeadcountForSize maps the app's company-size values ("51-200",
// "10001+") to the Sales Navigator bucket. Sales Navigator accepts only
// these bounds: min 1, 11, 51, 201, 501, 1001, 5001, 10001 and max 10,
// 50, 200, 500, 1000, 5000, 10000. ok is false for any other value.
func HeadcountForSize(size string) (h Headcount, ok bool) {
	switch strings.TrimSpace(size) {
	case "1-10":
		return Headcount{Min: 1, Max: 10}, true
	case "11-50":
		return Headcount{Min: 11, Max: 50}, true
	case "51-200":
		return Headcount{Min: 51, Max: 200}, true
	case "201-500":
		return Headcount{Min: 201, Max: 500}, true
	case "501-1000":
		return Headcount{Min: 501, Max: 1000}, true
	case "1001-5000":
		return Headcount{Min: 1001, Max: 5000}, true
	case "5001-10000":
		return Headcount{Min: 5001, Max: 10000}, true
	case "10001+":
		return Headcount{Min: 10001}, true
	}
	return Headcount{}, false
}

// PeopleSearch is a Sales Navigator people search. Each field narrows
// the search; the values inside one field are alternatives.
type PeopleSearch struct {
	// Titles are current job titles, in plain text.
	Titles []string
	// RegionIDs and IndustryIDs come from SearchParameters.
	RegionIDs   []string
	IndustryIDs []string
	// Headcount is the company size; nil means any size.
	Headcount *Headcount
	// Cursor continues an earlier search; "" asks for the first page.
	Cursor string
	// Limit is the page size.
	Limit int
}

// SearchPerson is one person on a result page.
type SearchPerson struct {
	Name             string
	FirstName        string
	LastName         string
	PublicIdentifier string
	// ProfileURL is https://www.linkedin.com/in/<public identifier>, or ""
	// when LinkedIn hides the profile (no public identifier). The rail
	// can invite only a person with a profile URL.
	ProfileURL string
	Headline   string
	Location   string
	// Title, Company and CompanyID come from the first current position.
	Title     string
	Company   string
	CompanyID string
	// NetworkDistance is LinkedIn's degree: DISTANCE_2, DISTANCE_3,
	// OUT_OF_NETWORK, …
	NetworkDistance string
	// PendingInvitation is true when this account already invited the
	// person.
	PendingInvitation bool
}

// PeoplePage is one page of a people search.
type PeoplePage struct {
	People []SearchPerson
	// Cursor fetches the next page; "" when there is none.
	Cursor string
	// Total is LinkedIn's result count when it gives one, else 0.
	Total int
}

type includeFilter struct {
	Include []string `json:"include"`
}

type headcountBucket struct {
	Min int `json:"min"`
	Max int `json:"max,omitempty"`
}

type peopleSearchRequest struct {
	API              string            `json:"api"`
	Category         string            `json:"category"`
	Role             *includeFilter    `json:"role,omitempty"`
	Location         *includeFilter    `json:"location,omitempty"`
	Industry         *includeFilter    `json:"industry,omitempty"`
	CompanyHeadcount []headcountBucket `json:"company_headcount,omitempty"`
	NetworkDistance  []int             `json:"network_distance"`
}

type peopleSearchResponse struct {
	Items  []peopleSearchItem `json:"items"`
	Cursor string             `json:"cursor"`
	Paging struct {
		TotalCount *int `json:"total_count"`
	} `json:"paging"`
}

type peopleSearchItem struct {
	Name              string `json:"name"`
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	PublicIdentifier  string `json:"public_identifier"`
	Headline          string `json:"headline"`
	Location          string `json:"location"`
	NetworkDistance   string `json:"network_distance"`
	PendingInvitation bool   `json:"pending_invitation"`
	CurrentPositions  []struct {
		Company   string    `json:"company"`
		CompanyID idOrEmpty `json:"company_id"`
		Role      string    `json:"role"`
	} `json:"current_positions"`
}

// idOrEmpty reads an id that Unipile may send as a string, a number or
// null.
type idOrEmpty string

func (s *idOrEmpty) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = idOrEmpty(str)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("unipile: id: unexpected JSON %s", b)
	}
	*s = idOrEmpty(n.String())
	return nil
}

// SearchPeople runs one page of a Sales Navigator people search as the
// connected account:
//
//	POST /api/v1/linkedin/search?account_id=<acc>&limit=<n>[&cursor=<c>]
//	{"api":"sales_navigator","category":"people","role":{"include":[…]},…}
//
// It leaves out 1st-degree connections (network_distance 2 and 3 only):
// the account cannot invite them. A restriction payload returns
// ErrAccountRestricted, as a send does.
func (m *Module) SearchPeople(ctx context.Context, accountID string, s PeopleSearch) (PeoplePage, error) {
	if !m.Configured() {
		return PeoplePage{}, ErrNotConfigured
	}
	req := peopleSearchRequest{
		API:             "sales_navigator",
		Category:        "people",
		NetworkDistance: []int{2, 3},
	}
	if titles := nonEmpty(s.Titles); len(titles) > 0 {
		req.Role = &includeFilter{Include: titles}
	}
	if ids := nonEmpty(s.RegionIDs); len(ids) > 0 {
		req.Location = &includeFilter{Include: ids}
	}
	if ids := nonEmpty(s.IndustryIDs); len(ids) > 0 {
		req.Industry = &includeFilter{Include: ids}
	}
	if s.Headcount != nil {
		req.CompanyHeadcount = []headcountBucket{{Min: s.Headcount.Min, Max: s.Headcount.Max}}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return PeoplePage{}, fmt.Errorf("unipile: marshal search request: %w", err)
	}

	q := url.Values{}
	q.Set("account_id", accountID)
	if s.Limit > 0 {
		q.Set("limit", strconv.Itoa(s.Limit))
	}
	if s.Cursor != "" {
		q.Set("cursor", s.Cursor)
	}
	raw, err := m.searchCall(ctx, http.MethodPost, "/api/v1/linkedin/search?"+q.Encode(), body)
	if err != nil {
		return PeoplePage{}, err
	}

	var r peopleSearchResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return PeoplePage{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	page := PeoplePage{People: make([]SearchPerson, 0, len(r.Items))}
	for _, it := range r.Items {
		p := SearchPerson{
			Name:              strings.TrimSpace(it.Name),
			FirstName:         strings.TrimSpace(it.FirstName),
			LastName:          strings.TrimSpace(it.LastName),
			PublicIdentifier:  strings.TrimSpace(it.PublicIdentifier),
			Headline:          strings.TrimSpace(it.Headline),
			Location:          strings.TrimSpace(it.Location),
			NetworkDistance:   it.NetworkDistance,
			PendingInvitation: it.PendingInvitation,
		}
		if p.PublicIdentifier != "" {
			p.ProfileURL = "https://www.linkedin.com/in/" + p.PublicIdentifier
		}
		if len(it.CurrentPositions) > 0 {
			pos := it.CurrentPositions[0]
			p.Title = strings.TrimSpace(pos.Role)
			p.Company = strings.TrimSpace(pos.Company)
			p.CompanyID = strings.TrimSpace(string(pos.CompanyID))
		}
		page.People = append(page.People, p)
	}
	if len(r.Items) > 0 {
		page.Cursor = r.Cursor
	}
	if r.Paging.TotalCount != nil {
		page.Total = *r.Paging.TotalCount
	}
	return page, nil
}

// searchCall runs one search request. It classifies the response as a
// send does: a search runs as the account, so a restriction payload
// must read as ErrAccountRestricted, not as a generic upstream error.
func (m *Module) searchCall(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	resp, err := m.do(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if err := classifySendStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
