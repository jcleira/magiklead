//go:build integration

package linkedinsearch_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	linkedinsearch "github.com/jcleira/magiklead/backend/internal/leads/linkedin_search"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/leads/linkedin_search/...

func withPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type stubServer struct {
	*httptest.Server
	calls int
}

func newStub(t *testing.T, status int, body []byte) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func uniqueSuffix() string { return "-" + uuid.NewString()[:8] }

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// searchBody builds a stubbed rockapis linkedin-data-api search response
// of the given profiles.
func searchBody(t *testing.T, items []map[string]any) []byte {
	return mustJSON(t, map[string]any{
		"success": true,
		"data":    map[string]any{"items": items},
	})
}

// TestSearch_WriteThrough is the tracer: a stubbed RapidAPI response of
// N profiles flows through Search → write-through → the canonical graph,
// landing N persons + N linkedin_url identifiers + N orgs + N current
// employments and ZERO emails (the email-less LinkedIn path).
func TestSearch_WriteThrough(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	profileURL := "https://www.linkedin.com/in/timcook" + strings.TrimPrefix(suffix, "-")
	body := searchBody(t, []map[string]any{
		{
			"fullName":      "Tim Cook" + suffix,
			"firstName":     "Tim",
			"lastName":      "Cook",
			"headline":      "Chief Executive Officer",
			"profileURL":    profileURL,
			"companyName":   "Apple Inc" + suffix,
			"companyDomain": "apple-" + strings.TrimPrefix(suffix, "-") + ".test",
			"location":      "Cupertino, California, United States",
		},
	})
	stub := newStub(t, 200, body)

	m := linkedinsearch.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	results, err := m.Search(ctx, linkedinsearch.Filters{Titles: []string{"CEO"}, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d, want 1", len(results))
	}
	got := results[0]
	if got.Name != "Tim Cook"+suffix {
		t.Errorf("Name=%q", got.Name)
	}
	if got.Title != "Chief Executive Officer" {
		t.Errorf("Title=%q", got.Title)
	}
	if got.OrganizationName != "Apple Inc"+suffix {
		t.Errorf("OrganizationName=%q", got.OrganizationName)
	}
	if got.LinkedInURL != profileURL {
		t.Errorf("LinkedInURL=%q want %q", got.LinkedInURL, profileURL)
	}

	// Canonical rows must now exist...
	assertCount(t, pool, 1, `SELECT count(*) FROM persons WHERE canonical_name=$1`, "Tim Cook"+suffix)
	assertCount(t, pool, 1, `SELECT count(*) FROM organizations WHERE primary_domain=$1`, "apple-"+strings.TrimPrefix(suffix, "-")+".test")
	// ...keyed on the linkedin_url identifier...
	assertCount(t, pool, 1, `SELECT count(*) FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1`, profileURL)
	// ...with a current employment...
	assertCount(t, pool, 1, `SELECT count(*) FROM employments e JOIN persons p ON p.id=e.person_id WHERE p.canonical_name=$1 AND e.is_current=TRUE`, "Tim Cook"+suffix)
	// ...and NO email anywhere on this path (AC #4).
	assertCount(t, pool, 0, `SELECT count(*) FROM emails em JOIN persons p ON p.id=em.person_id WHERE p.canonical_name=$1`, "Tim Cook"+suffix)
	// has_email stays false.
	assertCount(t, pool, 1, `SELECT count(*) FROM persons WHERE canonical_name=$1 AND has_email=FALSE`, "Tim Cook"+suffix)
}

// TestSearch_Idempotent re-ingests the same LinkedIn URL and asserts the
// write-through updates in place — one person, one identifier, one
// employment, no duplicates and still no email (AC #3 + #4).
func TestSearch_Idempotent(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	profileURL := "https://www.linkedin.com/in/janedoe" + strings.TrimPrefix(suffix, "-")
	body := searchBody(t, []map[string]any{
		{
			"fullName":      "Jane Doe" + suffix,
			"firstName":     "Jane",
			"lastName":      "Doe",
			"headline":      "VP Marketing",
			"profileURL":    profileURL,
			"companyName":   "Initech" + suffix,
			"companyDomain": "initech-" + strings.TrimPrefix(suffix, "-") + ".test",
		},
	})
	stub := newStub(t, 200, body)
	m := linkedinsearch.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	first, err := m.Search(ctx, linkedinsearch.Filters{Titles: []string{"VP Marketing"}})
	if err != nil {
		t.Fatalf("Search #1: %v", err)
	}
	second, err := m.Search(ctx, linkedinsearch.Filters{Titles: []string{"VP Marketing"}})
	if err != nil {
		t.Fatalf("Search #2: %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("results first=%d second=%d, want 1 each", len(first), len(second))
	}
	if first[0].PersonID != second[0].PersonID {
		t.Errorf("PersonID drift: first=%s second=%s", first[0].PersonID, second[0].PersonID)
	}

	assertCount(t, pool, 1, `SELECT count(*) FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1`, profileURL)
	assertCount(t, pool, 1, `SELECT count(*) FROM persons WHERE canonical_name=$1`, "Jane Doe"+suffix)
	assertCount(t, pool, 1, `SELECT count(*) FROM employments e JOIN persons p ON p.id=e.person_id WHERE p.canonical_name=$1`, "Jane Doe"+suffix)
	assertCount(t, pool, 0, `SELECT count(*) FROM emails em JOIN persons p ON p.id=em.person_id WHERE p.canonical_name=$1`, "Jane Doe"+suffix)
}

// TestSearch_CanonicalizesURL feeds a deliberately messy profileURL through
// the write-through and asserts the stored identifier is the single canonical
// form (via liurl) — not the raw provider spelling — so an ingest-planted or
// seed-planted row for the same person dedups against it on the exact string.
func TestSearch_CanonicalizesURL(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()
	slug := "canon" + strings.TrimPrefix(suffix, "-")

	// Uppercase scheme + host, trailing slash, query string.
	rawURL := "HTTP://WWW.LinkedIn.COM/in/" + slug + "/?trk=abc"
	wantURL := "https://www.linkedin.com/in/" + slug

	body := searchBody(t, []map[string]any{
		{
			"fullName":      "Canon Case" + suffix,
			"firstName":     "Canon",
			"lastName":      "Case",
			"headline":      "Engineer",
			"profileURL":    rawURL,
			"companyName":   "Canon Co" + suffix,
			"companyDomain": "canon-" + strings.TrimPrefix(suffix, "-") + ".test",
		},
	})
	stub := newStub(t, 200, body)
	m := linkedinsearch.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	results, err := m.Search(ctx, linkedinsearch.Filters{Titles: []string{"Engineer"}, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d, want 1", len(results))
	}
	if results[0].LinkedInURL != wantURL {
		t.Errorf("returned LinkedInURL=%q want canonical %q", results[0].LinkedInURL, wantURL)
	}

	// Stored under the canonical form; the raw messy spelling is never persisted.
	assertCount(t, pool, 1, `SELECT count(*) FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1`, wantURL)
	assertCount(t, pool, 0, `SELECT count(*) FROM person_identifiers WHERE identifier_value=$1`, rawURL)
}

func assertCount(t *testing.T, pool *pgxpool.Pool, want int, query string, args ...any) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Errorf("count=%d want %d for %q args=%v", got, want, query, args)
	}
}

// cleanupPeople deletes the persons stored for the given LinkedIn URLs
// (identifiers and jobs cascade) and the organizations they worked at.
func cleanupPeople(t *testing.T, pool *pgxpool.Pool, urls ...string) {
	t.Cleanup(func() {
		ctx := context.Background()
		for _, u := range urls {
			_, _ = pool.Exec(ctx, `
				WITH p AS (SELECT person_id FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1),
				     o AS (SELECT organization_id FROM employments WHERE person_id IN (SELECT person_id FROM p)),
				     dp AS (DELETE FROM persons WHERE id IN (SELECT person_id FROM p))
				DELETE FROM organizations WHERE id IN (SELECT organization_id FROM o)`, u)
		}
	})
}

// A LinkedIn search result carries no company domain. Writing the same
// profile again — the founder runs the same search twice — must reuse
// the company it was stored with: one person, one organization, one
// current job, with the new title.
func TestWriteThrough_ReusesCompanyWithoutDomain(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()
	profileURL := "https://www.linkedin.com/in/grace" + strings.TrimPrefix(suffix, "-")
	cleanupPeople(t, pool, profileURL)
	m := linkedinsearch.New(pool, "", nil)

	profile := linkedinsearch.Profile{
		FullName:    "Grace Hopper" + suffix,
		FirstName:   "Grace",
		LastName:    "Hopper",
		Title:       "Partner",
		CompanyName: "Hopper Law" + suffix,
		LinkedInURL: profileURL,
		Location:    "Arlington, Virginia, United States",
	}
	first, err := m.WriteThrough(ctx, []linkedinsearch.Profile{profile})
	if err != nil || len(first) != 1 {
		t.Fatalf("WriteThrough #1: %v (%d people)", err, len(first))
	}
	profile.Title = "Managing Partner"
	profile.CompanyName = "HOPPER LAW" + strings.ToUpper(suffix)
	second, err := m.WriteThrough(ctx, []linkedinsearch.Profile{profile})
	if err != nil || len(second) != 1 {
		t.Fatalf("WriteThrough #2: %v (%d people)", err, len(second))
	}

	if first[0].PersonID != second[0].PersonID || first[0].OrganizationID != second[0].OrganizationID {
		t.Errorf("person/org drift: first=%s/%s second=%s/%s",
			first[0].PersonID, first[0].OrganizationID, second[0].PersonID, second[0].OrganizationID)
	}
	assertCount(t, pool, 1, `SELECT count(*) FROM employments WHERE person_id=$1 AND is_current`, first[0].PersonID)
	assertCount(t, pool, 1, `SELECT count(*) FROM employments WHERE person_id=$1 AND is_current AND title='Managing Partner'`, first[0].PersonID)
}

// LinkedIn's company id keys the organization the way the ingest does,
// so two people at the same company share one organization even with
// no domain.
func TestWriteThrough_CompanyLinkedInIDKeysOrganization(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()
	companyID := "9" + strings.TrimPrefix(suffix, "-")
	urlA := "https://www.linkedin.com/in/ada" + strings.TrimPrefix(suffix, "-")
	urlB := "https://www.linkedin.com/in/alan" + strings.TrimPrefix(suffix, "-")
	cleanupPeople(t, pool, urlA, urlB)
	m := linkedinsearch.New(pool, "", nil)

	people, err := m.WriteThrough(ctx, []linkedinsearch.Profile{
		{FullName: "Ada Lovelace" + suffix, Title: "Managing Partner", CompanyName: "Engine LLP", CompanyLinkedInID: companyID, LinkedInURL: urlA},
		{FullName: "Alan Turing" + suffix, Title: "Partner", CompanyName: "Engine LLP", CompanyLinkedInID: companyID, LinkedInURL: urlB},
	})
	if err != nil || len(people) != 2 {
		t.Fatalf("WriteThrough: %v (%d people)", err, len(people))
	}
	if people[0].OrganizationID != people[1].OrganizationID {
		t.Errorf("two people at LinkedIn company %s got two organizations: %s and %s", companyID, people[0].OrganizationID, people[1].OrganizationID)
	}
	assertCount(t, pool, 1, `SELECT count(*) FROM organization_identifiers WHERE identifier_type='linkedin' AND identifier_value=$1`,
		"linkedin.com/company/"+companyID)
}
