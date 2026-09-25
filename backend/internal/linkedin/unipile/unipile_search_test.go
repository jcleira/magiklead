package unipile_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
)

// salesNavPage is a Sales Navigator people page in the shape Unipile
// documents (docs/linkedin-search): a named person with a public
// identifier and a current position, and a person LinkedIn hides (no
// name, no public identifier), as a classic search returned for all 10
// results in the 2026-09-25 capture.
const salesNavPage = `{
  "object": "LinkedinSearch",
  "items": [
    {
      "type": "PEOPLE",
      "id": "ACwAAtestsalesnav01",
      "name": "Ada Lovelace",
      "first_name": "Ada",
      "last_name": "Lovelace",
      "member_urn": "urn:li:member:111",
      "public_identifier": "ada-lovelace-test01",
      "public_profile_url": "https://www.linkedin.com/in/ada-lovelace-test01",
      "profile_url": "https://www.linkedin.com/sales/lead/ACwAAtestsalesnav01",
      "network_distance": "DISTANCE_3",
      "location": "New York, New York, United States",
      "headline": "Managing Partner at Lovelace & Babbage LLP",
      "pending_invitation": false,
      "premium": true,
      "open_profile": false,
      "current_positions": [
        {"company": "Lovelace & Babbage LLP", "company_id": 1441, "role": "Managing Partner", "tenure_at_role": {"years": 4}}
      ]
    },
    {
      "type": "PEOPLE",
      "id": "ACwAAtesthidden02",
      "name": "LinkedIn Member",
      "public_identifier": null,
      "network_distance": "OUT_OF_NETWORK",
      "location": "United States",
      "headline": "Managing Partner at Hidden LLP",
      "current_positions": []
    }
  ],
  "paging": {"start": 0, "page_count": 2, "total_count": 1234},
  "cursor": "next-page-cursor"
}`

func TestSearchPeople_RequestAndParse(t *testing.T) {
	s := newStub(t, 200, salesNavPage)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	page, err := m.SearchPeople(context.Background(), "acc_1", unipile.PeopleSearch{
		Titles:      []string{"Managing Partner", " ", "Founding Partner"},
		RegionIDs:   []string{"103644278"},
		IndustryIDs: []string{"10"},
		Headcount:   &unipile.Headcount{Min: 51, Max: 200},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchPeople: %v", err)
	}

	if s.method != http.MethodPost || s.path != "/api/v1/linkedin/search" {
		t.Errorf("request=%s %s want POST /api/v1/linkedin/search", s.method, s.path)
	}
	q, _ := url.ParseQuery(s.rawQuery)
	if q.Get("account_id") != "acc_1" || q.Get("limit") != "10" || q.Has("cursor") {
		t.Errorf("query=%q want account_id=acc_1&limit=10 and no cursor", s.rawQuery)
	}
	var body map[string]any
	if err := json.Unmarshal(s.lastBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	want := map[string]any{
		"api":               "sales_navigator",
		"category":          "people",
		"role":              map[string]any{"include": []any{"Managing Partner", "Founding Partner"}},
		"location":          map[string]any{"include": []any{"103644278"}},
		"industry":          map[string]any{"include": []any{"10"}},
		"company_headcount": []any{map[string]any{"min": float64(51), "max": float64(200)}},
		"network_distance":  []any{float64(2), float64(3)},
	}
	if !reflect.DeepEqual(body, want) {
		t.Errorf("body=%s\nwant %v", s.lastBody, want)
	}

	if page.Cursor != "next-page-cursor" || page.Total != 1234 {
		t.Errorf("cursor=%q total=%d want next-page-cursor/1234", page.Cursor, page.Total)
	}
	if len(page.People) != 2 {
		t.Fatalf("people=%d want 2", len(page.People))
	}
	ada := page.People[0]
	wantAda := unipile.SearchPerson{
		Name:             "Ada Lovelace",
		FirstName:        "Ada",
		LastName:         "Lovelace",
		PublicIdentifier: "ada-lovelace-test01",
		ProfileURL:       "https://www.linkedin.com/in/ada-lovelace-test01",
		Headline:         "Managing Partner at Lovelace & Babbage LLP",
		Location:         "New York, New York, United States",
		Title:            "Managing Partner",
		Company:          "Lovelace & Babbage LLP",
		CompanyID:        "1441",
		NetworkDistance:  "DISTANCE_3",
	}
	if ada != wantAda {
		t.Errorf("person=%+v\nwant   %+v", ada, wantAda)
	}
	if hidden := page.People[1]; hidden.ProfileURL != "" || hidden.PublicIdentifier != "" {
		t.Errorf("hidden person has profile %q / %q; want none (LinkedIn hides it)", hidden.ProfileURL, hidden.PublicIdentifier)
	}
}

// A page with no filters but the network degree, and a cursor, sends the
// cursor and leaves every empty filter out of the body.
func TestSearchPeople_CursorAndEmptyFilters(t *testing.T) {
	s := newStub(t, 200, `{"object":"LinkedinSearch","items":[],"paging":{"total_count":null},"cursor":"stale"}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	page, err := m.SearchPeople(context.Background(), "acc_1", unipile.PeopleSearch{Cursor: "abc=="})
	if err != nil {
		t.Fatalf("SearchPeople: %v", err)
	}
	q, _ := url.ParseQuery(s.rawQuery)
	if q.Get("cursor") != "abc==" {
		t.Errorf("cursor=%q want abc==", q.Get("cursor"))
	}
	var body map[string]any
	if err := json.Unmarshal(s.lastBody, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for _, key := range []string{"role", "location", "industry", "company_headcount"} {
		if _, ok := body[key]; ok {
			t.Errorf("body=%s carries %q; an empty filter must be left out", s.lastBody, key)
		}
	}
	if page.Cursor != "" || page.Total != 0 || len(page.People) != 0 {
		t.Errorf("empty page=%+v want no people, no cursor, no total", page)
	}
}

func TestSearchPeople_ClassifiesErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", 401, `{}`, unipile.ErrUnauthorized},
		{"rate limited", 429, `{}`, unipile.ErrRateLimited},
		{"restricted", 403, `{"type":"errors/account_restricted","detail":"The account is restricted"}`, unipile.ErrAccountRestricted},
		{"no sales navigator", 400, `{"type":"errors/invalid_parameters","detail":"Sales Navigator is not available"}`, unipile.ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.status, tc.body)
			m := unipile.New("key", s.URL, nil, s.Client())
			m.SetBaseURL(s.URL)
			_, err := m.SearchPeople(context.Background(), "acc_1", unipile.PeopleSearch{Titles: []string{"CEO"}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
			if tc.want == unipile.ErrUpstream && !strings.Contains(err.Error(), "Sales Navigator is not available") {
				t.Errorf("err=%q must carry Unipile's detail for the UI", err)
			}
		})
	}
}

func TestSearchParameters(t *testing.T) {
	s := newStub(t, 200, `{"object":"LinkedinSearchParametersList","items":[
		{"object":"LinkedinSearchParameter","title":"United States","id":"103644278"},
		{"object":"LinkedinSearchParameter","title":"no id","id":""},
		{"object":"LinkedinSearchParameter","title":"Texas, United States","id":"102748797"}],"paging":{"page_count":3}}`)
	m := unipile.New("key", s.URL, nil, s.Client())
	m.SetBaseURL(s.URL)

	got, err := m.SearchParameters(context.Background(), "acc_1", unipile.ParamRegion, "United States", 5)
	if err != nil {
		t.Fatalf("SearchParameters: %v", err)
	}
	if s.method != http.MethodGet || s.path != "/api/v1/linkedin/search/parameters" {
		t.Errorf("request=%s %s", s.method, s.path)
	}
	q, _ := url.ParseQuery(s.rawQuery)
	if q.Get("account_id") != "acc_1" || q.Get("type") != "REGION" || q.Get("keywords") != "United States" || q.Get("limit") != "5" {
		t.Errorf("query=%q", s.rawQuery)
	}
	want := []unipile.SearchParameter{{ID: "103644278", Title: "United States"}, {ID: "102748797", Title: "Texas, United States"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("params=%+v want %+v", got, want)
	}
}

func TestSearch_NotConfigured(t *testing.T) {
	m := unipile.New("", "https://dsn.test", nil, nil)
	if _, err := m.SearchPeople(context.Background(), "acc", unipile.PeopleSearch{}); !errors.Is(err, unipile.ErrNotConfigured) {
		t.Errorf("SearchPeople err=%v want ErrNotConfigured", err)
	}
	if _, err := m.SearchParameters(context.Background(), "acc", unipile.ParamRegion, "x", 1); !errors.Is(err, unipile.ErrNotConfigured) {
		t.Errorf("SearchParameters err=%v want ErrNotConfigured", err)
	}
}

func TestHeadcountForSize(t *testing.T) {
	cases := map[string]unipile.Headcount{
		"1-10":       {Min: 1, Max: 10},
		"11-50":      {Min: 11, Max: 50},
		"51-200":     {Min: 51, Max: 200},
		"201-500":    {Min: 201, Max: 500},
		"501-1000":   {Min: 501, Max: 1000},
		"1001-5000":  {Min: 1001, Max: 5000},
		"5001-10000": {Min: 5001, Max: 10000},
		"10001+":     {Min: 10001},
	}
	for size, want := range cases {
		got, ok := unipile.HeadcountForSize(size)
		if !ok || got != want {
			t.Errorf("HeadcountForSize(%q)=%+v,%v want %+v,true", size, got, ok, want)
		}
	}
	if _, ok := unipile.HeadcountForSize("12-34"); ok {
		t.Error("an unknown size must not map to a bucket")
	}
}
