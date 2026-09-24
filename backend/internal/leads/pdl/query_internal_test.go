package pdl

import (
	"encoding/json"
	"testing"
)

// TestBuildESQuery_ValuesInADimensionAreAlternatives pins the query shape
// PDL accepted live (HTTP 200, 2,759 matches for the smoke's play #1): one
// must clause per dimension, the values inside it ORed. The old shape put
// every title and every location in its own must clause, so a search for
// two titles or two countries matched nobody and PDL answered 404.
func TestBuildESQuery_ValuesInADimensionAreAlternatives(t *testing.T) {
	got := buildESQuery(Filters{
		Titles:      []string{"Content Creator", " Executive Coach ", ""},
		Industries:  []string{"E-Commerce", "Retail"},
		CompanySize: "1-10",
		Locations:   []string{"United States", "Canada"},
		Description: "personal brand",
	})
	want := map[string]any{"bool": map[string]any{"must": []map[string]any{
		{"bool": map[string]any{"should": []map[string]any{
			{"match_phrase": map[string]any{"job_title": "Content Creator"}},
			{"match_phrase": map[string]any{"job_title": "Executive Coach"}},
		}}},
		{"terms": map[string]any{"job_company_industry": []string{"e-commerce", "retail"}}},
		{"term": map[string]any{"job_company_size": "1-10"}},
		{"bool": map[string]any{"should": []map[string]any{
			{"match_phrase": map[string]any{"location_name": "United States"}},
			{"match_phrase": map[string]any{"location_name": "Canada"}},
		}}},
		{"query_string": map[string]any{"query": "personal brand"}},
	}}}
	assertSameJSON(t, got, want)
}

// TestBuildESQuery_NoFiltersMatchesAnyEmailable keeps the "any record"
// default: blank values add no clause, and PDL needs at least one.
func TestBuildESQuery_NoFiltersMatchesAnyEmailable(t *testing.T) {
	got := buildESQuery(Filters{Titles: []string{" "}, Locations: []string{""}})
	want := map[string]any{"bool": map[string]any{"must": []map[string]any{
		{"exists": map[string]any{"field": "work_email"}},
	}}}
	assertSameJSON(t, got, want)
}

// assertSameJSON compares the wire form: json.Marshal sorts map keys, so
// equal queries encode to equal bytes.
func assertSameJSON(t *testing.T, got, want any) {
	t.Helper()
	g, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	w, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if string(g) != string(w) {
		t.Errorf("query=%s\n want=%s", g, w)
	}
}
