//go:build integration

package pdl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/leads/pdl"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/leads/pdl/...

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

// stubServer wraps an httptest.Server with a counter so tests can
// assert "did we actually hit PDL?".
type stubServer struct {
	*httptest.Server
	calls    int
	lastBody []byte
}

func newStub(t *testing.T, status int, body []byte) *stubServer {
	t.Helper()
	s := &stubServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.lastBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

// uniqueSuffix scopes fixture identifiers per-test so concurrent runs
// don't collide on the unique (email) / unique (identifier_value)
// constraints.
func uniqueSuffix() string { return "-" + uuid.NewString()[:8] }

func TestSearch_WriteThrough(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	body := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{
			{
				"id":                   "pdl-tim" + suffix,
				"full_name":            "Tim Cook" + suffix,
				"first_name":           "Tim",
				"last_name":            "Cook",
				"job_title":            "Chief Executive Officer",
				"job_company_name":     "Apple Inc",
				"job_company_website":  "apple-" + strings.TrimPrefix(suffix, "-") + ".test",
				"job_company_industry": "Computer Hardware",
				"job_company_size":     "10001+",
				"location_name":        "Cupertino, California, United States",
				"work_email":           "tim" + suffix + "@apple.test",
			},
		},
	})

	stub := newStub(t, 200, body)

	m := pdl.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	results, err := m.Search(ctx, pdl.Filters{
		Industries: []string{"Computer Hardware"},
		Limit:      10,
	})
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
	if got.Email != "tim"+suffix+"@apple.test" {
		t.Errorf("Email=%q", got.Email)
	}
	if !got.EmailVerified {
		t.Errorf("EmailVerified should be true (pdl-verified path)")
	}
	if got.Title != "Chief Executive Officer" {
		t.Errorf("Title=%q", got.Title)
	}
	if got.OrganizationName != "Apple Inc" {
		t.Errorf("OrganizationName=%q", got.OrganizationName)
	}
	if got.SizeRange != "10001+" {
		t.Errorf("SizeRange=%q", got.SizeRange)
	}

	// Canonical rows must now exist.
	assertPersonRow(t, pool, got.PersonID)
	assertOrgRow(t, pool, got.OrganizationID)
	assertEmploymentLinking(t, pool, got.PersonID, got.OrganizationID)
	assertEmailRow(t, pool, "tim"+suffix+"@apple.test", "pdl-verified")
}

func TestSearch_Idempotent(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	body := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{
			{
				"id":                   "pdl-jane" + suffix,
				"full_name":            "Jane Doe" + suffix,
				"first_name":           "Jane",
				"last_name":            "Doe",
				"job_title":            "VP Marketing",
				"job_company_name":     "Initech",
				"job_company_website":  "initech-" + strings.TrimPrefix(suffix, "-") + ".test",
				"job_company_industry": "Software",
				"job_company_size":     "201-500",
				"location_name":        "San Francisco, California, United States",
				"work_email":           "jane" + suffix + "@initech.test",
			},
		},
	})
	stub := newStub(t, 200, body)

	m := pdl.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	first, err := m.Search(ctx, pdl.Filters{Titles: []string{"VP Marketing"}})
	if err != nil {
		t.Fatalf("Search #1: %v", err)
	}
	second, err := m.Search(ctx, pdl.Filters{Titles: []string{"VP Marketing"}})
	if err != nil {
		t.Fatalf("Search #2: %v", err)
	}

	if first[0].PersonID != second[0].PersonID {
		t.Errorf("PersonID drift: first=%s second=%s", first[0].PersonID, second[0].PersonID)
	}
	if first[0].OrganizationID != second[0].OrganizationID {
		t.Errorf("OrganizationID drift: first=%s second=%s", first[0].OrganizationID, second[0].OrganizationID)
	}

	// Both calls hit the stub; that's expected for the module itself
	// (the handler is what decides "skip PDL if canonical fresh"). The
	// invariant we care about here is no row duplication.
	var personCount, orgCount, employmentCount, emailCount int
	pool.QueryRow(ctx,
		`SELECT count(*) FROM persons WHERE canonical_name=$1`,
		"Jane Doe"+suffix).Scan(&personCount)
	pool.QueryRow(ctx,
		`SELECT count(*) FROM organizations WHERE primary_domain=$1`,
		"initech-"+strings.TrimPrefix(suffix, "-")+".test").Scan(&orgCount)
	pool.QueryRow(ctx,
		`SELECT count(*) FROM employments e JOIN persons p ON p.id = e.person_id
		 WHERE p.canonical_name=$1`,
		"Jane Doe"+suffix).Scan(&employmentCount)
	pool.QueryRow(ctx,
		`SELECT count(*) FROM emails WHERE email=$1`,
		"jane"+suffix+"@initech.test").Scan(&emailCount)

	if personCount != 1 {
		t.Errorf("persons rows=%d, want 1", personCount)
	}
	if orgCount != 1 {
		t.Errorf("organizations rows=%d, want 1", orgCount)
	}
	if employmentCount != 1 {
		t.Errorf("employments rows=%d, want 1", employmentCount)
	}
	if emailCount != 1 {
		t.Errorf("emails rows=%d, want 1", emailCount)
	}
}

func TestSearch_FailurePaths(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		status int
		body   []byte
		want   error
	}{
		{"rate_limited", http.StatusTooManyRequests, []byte(`{}`), pdl.ErrRateLimited},
		{"credit_exhausted", http.StatusPaymentRequired, []byte(`{}`), pdl.ErrCreditExhausted},
		{"unauthorized", http.StatusUnauthorized, []byte(`{}`), pdl.ErrUnauthorized},
		{"malformed", http.StatusOK, []byte(`not json`), pdl.ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := newStub(t, tc.status, tc.body)
			m := pdl.New(pool, "dev", stub.Client())
			m.SetBaseURL(stub.URL)

			_, err := m.Search(ctx, pdl.Filters{Titles: []string{"CEO"}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v, want %v", err, tc.want)
			}
		})
	}
}

func TestSearch_NotConfigured(t *testing.T) {
	pool := withPool(t)
	m := pdl.New(pool, "", nil)
	_, err := m.Search(context.Background(), pdl.Filters{Titles: []string{"CEO"}})
	if !errors.Is(err, pdl.ErrNotConfigured) {
		t.Fatalf("err=%v, want ErrNotConfigured", err)
	}
}

func TestSearchWithCache_CacheHit(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	// Seed a fresh canonical row that matches the filter. Scope the
	// industry + title with the test suffix so accumulated rows from
	// prior `go test` runs don't pollute the result set.
	industry := "cache-hit-industry" + suffix
	title := "Director Sales " + suffix
	domain := "freshhit-" + strings.TrimPrefix(suffix, "-") + ".test"
	orgID, personID := seedFreshCanonical(t, pool, title, "VeryFresh Co", domain, industry, "11-50")

	// Stub should NOT be hit; if it is, the test fails on stub.calls.
	stub := newStub(t, 200, []byte(`{"data":[]}`))
	m := pdl.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	results, pdlCalled, err := m.SearchWithCache(ctx, pdl.Filters{
		Industries: []string{industry},
		Titles:     []string{title},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("SearchWithCache: %v", err)
	}
	if pdlCalled {
		t.Errorf("pdlCalled=true but cache should have served")
	}
	if stub.calls != 0 {
		t.Errorf("PDL was hit %d times despite a cache hit", stub.calls)
	}
	if len(results) != 1 {
		t.Fatalf("results=%d, want 1 (cached row)", len(results))
	}
	if results[0].PersonID != personID {
		t.Errorf("PersonID=%s, want cached %s", results[0].PersonID, personID)
	}
	if results[0].OrganizationID != orgID {
		t.Errorf("OrganizationID=%s, want cached %s", results[0].OrganizationID, orgID)
	}
}

func TestSearchWithCache_StaleCache(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	// Scope industry + title with the test suffix so prior accumulated
	// rows don't accidentally satisfy the freshness check.
	industry := "stale-industry" + suffix
	title := "VP Operations " + suffix
	domain := "stale-" + strings.TrimPrefix(suffix, "-") + ".test"
	_, _ = seedStaleCanonical(t, pool, title, "Stale Co", domain, industry, "501-1000", 100*24*time.Hour)

	// PDL response refreshes the row in place via write-through.
	body := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{
			{
				"id":                   "pdl-vp" + suffix,
				"full_name":            "Vera Patel" + suffix,
				"first_name":           "Vera",
				"last_name":            "Patel",
				"job_title":            title,
				"job_company_name":     "Stale Co",
				"job_company_website":  domain,
				"job_company_industry": industry,
				"job_company_size":     "501-1000",
				"location_name":        "Boston, Massachusetts, United States",
				"work_email":           "vera" + suffix + "@" + domain,
			},
		},
	})
	stub := newStub(t, 200, body)
	m := pdl.New(pool, "dev", stub.Client())
	m.SetBaseURL(stub.URL)

	results, pdlCalled, err := m.SearchWithCache(ctx, pdl.Filters{
		Industries: []string{industry},
		Titles:     []string{title},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("SearchWithCache: %v", err)
	}
	if !pdlCalled {
		t.Errorf("pdlCalled=false but cache was stale; PDL should have been called")
	}
	if stub.calls != 1 {
		t.Errorf("PDL hits=%d, want 1", stub.calls)
	}
	if len(results) == 0 {
		t.Fatalf("results=0 after PDL refresh")
	}

	// The PDL-written row's updated_at should now be within the freshness
	// window — re-querying with require_fresh=true (via the cache path)
	// must return the same row without hitting PDL.
	second, pdlCalledAgain, err := m.SearchWithCache(ctx, pdl.Filters{
		Industries: []string{industry},
		Titles:     []string{title},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("SearchWithCache #2: %v", err)
	}
	if pdlCalledAgain {
		t.Errorf("Second call hit PDL; canonical should be fresh after the first refresh")
	}
	if stub.calls != 1 {
		t.Errorf("PDL hits after second call=%d, want still 1", stub.calls)
	}
	if len(second) == 0 {
		t.Errorf("Second canonical lookup returned 0 rows")
	}
}

func TestEnrich_HappyPath(t *testing.T) {
	pool := withPool(t)
	ctx := context.Background()
	suffix := uniqueSuffix()

	// Seed via Search so the pdl_id identifier is wired.
	seedBody := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{
			{
				"id":                  "pdl-rich" + suffix,
				"full_name":           "Rich Reed" + suffix,
				"first_name":          "Rich",
				"last_name":           "Reed",
				"job_title":           "Director Engineering",
				"job_company_name":    "Acme Co",
				"job_company_website": "acme-" + strings.TrimPrefix(suffix, "-") + ".test",
			},
		},
	})
	seedStub := newStub(t, 200, seedBody)
	m := pdl.New(pool, "dev", seedStub.Client())
	m.SetBaseURL(seedStub.URL)

	results, err := m.Search(ctx, pdl.Filters{Titles: []string{"Director"}})
	if err != nil {
		t.Fatalf("Search seed: %v", err)
	}
	personID := results[0].PersonID

	enrichBody := mustJSON(t, map[string]any{
		"status": 200,
		"data": map[string]any{
			"work_email": "rich" + suffix + "@acme.test",
		},
	})
	enrichStub := newStub(t, 200, enrichBody)
	m.SetBaseURL(enrichStub.URL)

	email, err := m.Enrich(ctx, personID)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if email.Email != "rich"+suffix+"@acme.test" {
		t.Errorf("Email=%q", email.Email)
	}
	if !email.Verified {
		t.Errorf("Verified should be true")
	}
}

// ---- helpers --------------------------------------------------------------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func seedFreshCanonical(t *testing.T, pool *pgxpool.Pool, title, company, domain, industry, size string) (orgID, personID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID = uuid.New()
	personID = uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO organizations (id, canonical_name, primary_domain, industries, size_range, updated_at)
		 VALUES ($1, $2, $3, ARRAY[$4]::text[], $5, NOW())`,
		orgID, company, domain, strings.ToLower(industry), size); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO persons (id, canonical_name, first_name, last_name, normalized_name, updated_at)
		 VALUES ($1, $2, 'Cached', 'Person', LOWER($2), NOW())`,
		personID, "Cached Person "+uuid.NewString()); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO employments (person_id, organization_id, title, is_current)
		 VALUES ($1, $2, $3, TRUE)`, personID, orgID, title); err != nil {
		t.Fatalf("seed employment: %v", err)
	}
	return orgID, personID
}

func seedStaleCanonical(t *testing.T, pool *pgxpool.Pool, title, company, domain, industry, size string, age time.Duration) (orgID, personID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID = uuid.New()
	personID = uuid.New()
	staleTime := time.Now().Add(-age)
	if _, err := pool.Exec(ctx,
		`INSERT INTO organizations (id, canonical_name, primary_domain, industries, size_range, updated_at, created_at)
		 VALUES ($1, $2, $3, ARRAY[$4]::text[], $5, $6, $6)`,
		orgID, company, domain, strings.ToLower(industry), size, staleTime); err != nil {
		t.Fatalf("seed stale org: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO persons (id, canonical_name, first_name, last_name, normalized_name, updated_at, created_at)
		 VALUES ($1, $2, 'Stale', 'Person', LOWER($2), $3, $3)`,
		personID, "Stale Person "+uuid.NewString(), staleTime); err != nil {
		t.Fatalf("seed stale person: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO employments (person_id, organization_id, title, is_current)
		 VALUES ($1, $2, $3, TRUE)`, personID, orgID, title); err != nil {
		t.Fatalf("seed stale employment: %v", err)
	}
	return orgID, personID
}

func assertPersonRow(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM persons WHERE id=$1`, id).Scan(&count); err != nil {
		t.Fatalf("count person: %v", err)
	}
	if count != 1 {
		t.Errorf("person %s rows=%d", id, count)
	}
}

func assertOrgRow(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM organizations WHERE id=$1`, id).Scan(&count); err != nil {
		t.Fatalf("count org: %v", err)
	}
	if count != 1 {
		t.Errorf("org %s rows=%d", id, count)
	}
}

func assertEmploymentLinking(t *testing.T, pool *pgxpool.Pool, personID, orgID uuid.UUID) {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM employments
		 WHERE person_id=$1 AND organization_id=$2 AND is_current=TRUE`,
		personID, orgID).Scan(&count); err != nil {
		t.Fatalf("count employment: %v", err)
	}
	if count != 1 {
		t.Errorf("employment(person=%s, org=%s) rows=%d", personID, orgID, count)
	}
}

func assertEmailRow(t *testing.T, pool *pgxpool.Pool, email, wantMethod string) {
	t.Helper()
	var method string
	var verifiedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT verification_method, verified_at FROM emails WHERE email=$1`,
		email).Scan(&method, &verifiedAt); err != nil {
		t.Fatalf("read email: %v", err)
	}
	if method != wantMethod {
		t.Errorf("verification_method=%q, want %q", method, wantMethod)
	}
	if verifiedAt == nil {
		t.Errorf("verified_at must be set")
	}
}
