//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	linkedinsearch "github.com/jcleira/magiklead/backend/internal/leads/linkedin_search"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Run with:
//   DATABASE_URL=… go test -tags=integration ./internal/handler/...
//
// POST /leads/linkedin-search runs a Sales Navigator people search as
// the tenant's connected account (a stub Unipile here), writes the
// results through to the canonical graph and logs the search against
// the account's daily profile budget.

// unipileSearchStub answers the two search endpoints: the id lookup
// knows "United States" (REGION) and "Legal Services" (SALES_INDUSTRY),
// and the people search returns searchStatus/searchBody.
type unipileSearchStub struct {
	*httptest.Server
	searchStatus int
	searchBody   string

	mu          sync.Mutex
	searchCalls int
	searchQuery string
	searchReq   map[string]any
}

func newUnipileSearchStub(t *testing.T, searchStatus int, searchBody string) *unipileSearchStub {
	t.Helper()
	s := &unipileSearchStub{searchStatus: searchStatus, searchBody: searchBody}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/linkedin/search/parameters":
			known := map[string]string{
				"REGION|United States":          `{"id":"103644278","title":"United States"}`,
				"SALES_INDUSTRY|Legal Services": `{"id":"10","title":"Legal Services"}`,
			}
			item, ok := known[r.URL.Query().Get("type")+"|"+r.URL.Query().Get("keywords")]
			if !ok {
				_, _ = w.Write([]byte(`{"object":"LinkedinSearchParametersList","items":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"object":"LinkedinSearchParametersList","items":[` + item + `]}`))
		case "/api/v1/linkedin/search":
			raw, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.searchCalls++
			s.searchQuery = r.URL.RawQuery
			_ = json.Unmarshal(raw, &s.searchReq)
			s.mu.Unlock()
			w.WriteHeader(s.searchStatus)
			_, _ = w.Write([]byte(s.searchBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

// searchFixture is a tenant with one active LinkedIn account.
type searchFixture struct {
	tenantID  uuid.UUID
	accountID uuid.UUID
	unipileID string
}

func newSearchFixture(t *testing.T, pool *pgxpool.Pool, withAccount bool) searchFixture {
	t.Helper()
	run := uuid.NewString()
	f := searchFixture{tenantID: uuid.New(), accountID: uuid.New(), unipileID: "acc_search_" + run}
	metricsExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, f.tenantID, "LinkedIn Search Test "+run)
	if withAccount {
		metricsExec(t, pool, `INSERT INTO linkedin_accounts (id, tenant_id, unipile_account_id, status) VALUES ($1, $2, $3, 'active')`,
			f.accountID, f.tenantID, f.unipileID)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM linkedin_searches WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM linkedin_accounts WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
	})
	return f
}

// cleanupProfile deletes the person stored for a LinkedIn URL (its
// identifiers and jobs cascade) and the organizations it worked at.
func cleanupProfile(t *testing.T, pool *pgxpool.Pool, url string) {
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			WITH p AS (SELECT person_id FROM person_identifiers WHERE identifier_type='linkedin_url' AND identifier_value=$1),
			     o AS (SELECT organization_id FROM employments WHERE person_id IN (SELECT person_id FROM p)),
			     dp AS (DELETE FROM persons WHERE id IN (SELECT person_id FROM p))
			DELETE FROM organizations WHERE id IN (SELECT organization_id FROM o)`, url)
	})
}

func newTestSearchHandler(pool *pgxpool.Pool, stub *unipileSearchStub) *LinkedInSearchHandler {
	u := unipile.New("key", stub.URL, []byte("secret"), stub.Client())
	u.SetBaseURL(stub.URL)
	return NewLinkedInSearchHandler(repository.New(pool), u, linkedinsearch.New(pool, "", nil))
}

func callLinkedInSearch(t *testing.T, h *LinkedInSearchHandler, tenantID uuid.UUID, body string) (*httptest.ResponseRecorder, linkedInSearchResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/linkedin-search", strings.NewReader(body))
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()
	h.Search(rr, req)
	var resp linkedInSearchResponse
	if rr.Code == http.StatusOK {
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode %s: %v", rr.Body.String(), err)
		}
	}
	return rr, resp
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	return body["code"]
}

// The tracer: typed filters resolve to LinkedIn ids, the Sales Navigator
// search runs as the tenant's account, the named result lands in the
// canonical graph (LinkedIn URL, job, company keyed by LinkedIn id,
// location), the hidden result is only counted, and the search is logged
// against the account's budget.
func TestLinkedInSearch_WritesThroughAndLogs(t *testing.T) {
	pool := withMetricsPool(t)
	f := newSearchFixture(t, pool, true)
	slug := "ada-search-" + uuid.NewString()[:8]
	profile := "https://www.linkedin.com/in/" + slug
	cleanupProfile(t, pool, profile)
	companyID := "88" + uuid.NewString()[:6]
	stub := newUnipileSearchStub(t, 200, `{"object":"LinkedinSearch","items":[
		{"type":"PEOPLE","id":"ACwAAone","name":"Ada Lovelace","first_name":"Ada","last_name":"Lovelace",
		 "public_identifier":"`+slug+`","network_distance":"DISTANCE_3","location":"New York, New York, United States",
		 "headline":"Managing Partner at Lovelace LLP","pending_invitation":true,
		 "current_positions":[{"company":"Lovelace LLP","company_id":"`+companyID+`","role":"Managing Partner"}]},
		{"type":"PEOPLE","id":"ACwAAtwo","name":"LinkedIn Member","public_identifier":null,"network_distance":"OUT_OF_NETWORK"}],
		"paging":{"total_count":57},"cursor":"page-2"}`)
	h := newTestSearchHandler(pool, stub)

	rr, resp := callLinkedInSearch(t, h, f.tenantID,
		`{"titles":["Managing Partner"],"locations":["United States"],"industries":["Legal Services"],"company_size":"51-200"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	// The search Unipile got.
	if stub.searchCalls != 1 {
		t.Fatalf("search calls=%d want 1", stub.searchCalls)
	}
	if !strings.Contains(stub.searchQuery, "account_id="+f.unipileID) || !strings.Contains(stub.searchQuery, "limit=10") {
		t.Errorf("search query=%q want the account and limit=10", stub.searchQuery)
	}
	gotReq, _ := json.Marshal(stub.searchReq)
	for _, want := range []string{`"api":"sales_navigator"`, `"location":{"include":["103644278"]}`, `"industry":{"include":["10"]}`,
		`"role":{"include":["Managing Partner"]}`, `"company_headcount":[{"max":200,"min":51}]`} {
		if !strings.Contains(string(gotReq), want) {
			t.Errorf("search body=%s missing %s", gotReq, want)
		}
	}

	// The response.
	if resp.Count != 1 || resp.Hidden != 1 || resp.Cursor != "page-2" || resp.Total != 57 {
		t.Errorf("count=%d hidden=%d cursor=%q total=%d want 1/1/page-2/57", resp.Count, resp.Hidden, resp.Cursor, resp.Total)
	}
	if resp.UsedToday != 2 || resp.DailyCap != linkedInSearchDailyCap {
		t.Errorf("used_today=%d daily_cap=%d want 2/%d", resp.UsedToday, resp.DailyCap, linkedInSearchDailyCap)
	}
	if len(resp.Locations) != 1 || resp.Locations[0].ID != "103644278" || len(resp.Industries) != 1 || resp.Industries[0].ID != "10" {
		t.Errorf("matched locations=%+v industries=%+v", resp.Locations, resp.Industries)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("results=%+v want 1", resp.Results)
	}
	ada := resp.Results[0]
	if ada.LinkedInURL != profile || ada.Name != "Ada Lovelace" || ada.FirstName != "Ada" ||
		ada.Title != "Managing Partner" || ada.Company != "Lovelace LLP" || !ada.PendingInvitation {
		t.Errorf("result=%+v", ada)
	}

	// The canonical graph.
	var personID uuid.UUID
	var location, title, company string
	if err := pool.QueryRow(context.Background(), `
		SELECT p.id, p.location, e.title, o.canonical_name
		FROM person_identifiers pi
		JOIN persons p ON p.id = pi.person_id
		JOIN employments e ON e.person_id = p.id AND e.is_current
		JOIN organizations o ON o.id = e.organization_id
		WHERE pi.identifier_type = 'linkedin_url' AND pi.identifier_value = $1`, profile).
		Scan(&personID, &location, &title, &company); err != nil {
		t.Fatalf("stored person: %v", err)
	}
	if personID.String() != ada.PersonID || location != "New York, New York, United States" || title != "Managing Partner" || company != "Lovelace LLP" {
		t.Errorf("stored person=%s location=%q title=%q company=%q", personID, location, title, company)
	}
	var keyed int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM organization_identifiers WHERE identifier_type='linkedin' AND identifier_value=$1`,
		"linkedin.com/company/"+companyID).Scan(&keyed); err != nil || keyed != 1 {
		t.Errorf("company id identifier rows=%d err=%v want 1", keyed, err)
	}

	// The log row counts both profiles LinkedIn returned.
	var logged, sum int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*), COALESCE(SUM(result_count), 0) FROM linkedin_searches WHERE tenant_id = $1 AND linkedin_account_id = $2`,
		f.tenantID, f.accountID).Scan(&logged, &sum); err != nil {
		t.Fatalf("count searches: %v", err)
	}
	if logged != 1 || sum != 2 {
		t.Errorf("logged searches=%d profiles=%d want 1/2", logged, sum)
	}
}

// The budget: a spent account gets 429 and no search runs; a nearly
// spent one asks LinkedIn for only what is left.
func TestLinkedInSearch_DailyBudget(t *testing.T) {
	pool := withMetricsPool(t)
	f := newSearchFixture(t, pool, true)
	stub := newUnipileSearchStub(t, 200, `{"object":"LinkedinSearch","items":[],"paging":{},"cursor":null}`)
	h := newTestSearchHandler(pool, stub)

	metricsExec(t, pool, `INSERT INTO linkedin_searches (tenant_id, linkedin_account_id, filters, result_count) VALUES ($1, $2, '{}', $3)`,
		f.tenantID, f.accountID, linkedInSearchDailyCap-5)
	rr, _ := callLinkedInSearch(t, h, f.tenantID, `{"titles":["Managing Partner"]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(stub.searchQuery, "limit=5") {
		t.Errorf("search query=%q want limit=5 (what is left of the budget)", stub.searchQuery)
	}

	metricsExec(t, pool, `INSERT INTO linkedin_searches (tenant_id, linkedin_account_id, filters, result_count) VALUES ($1, $2, '{}', 5)`,
		f.tenantID, f.accountID)
	rr, _ = callLinkedInSearch(t, h, f.tenantID, `{"titles":["Managing Partner"]}`)
	if rr.Code != http.StatusTooManyRequests || errorCode(t, rr) != "linkedin_search_limit" {
		t.Errorf("status=%d code=%q want 429 linkedin_search_limit", rr.Code, errorCode(t, rr))
	}
	if stub.searchCalls != 1 {
		t.Errorf("search calls=%d want 1 (none once the budget is spent)", stub.searchCalls)
	}

	// A search older than 24 hours no longer counts.
	metricsExec(t, pool, `UPDATE linkedin_searches SET created_at = NOW() - INTERVAL '25 hours' WHERE tenant_id = $1`, f.tenantID)
	rr, _ = callLinkedInSearch(t, h, f.tenantID, `{"titles":["Managing Partner"]}`)
	if rr.Code != http.StatusOK {
		t.Errorf("status=%d want 200 after the window rolls", rr.Code)
	}
}

// Requests the handler rejects before any people search runs.
func TestLinkedInSearch_Rejects(t *testing.T) {
	pool := withMetricsPool(t)
	withAccount := newSearchFixture(t, pool, true)
	noAccount := newSearchFixture(t, pool, false)
	stub := newUnipileSearchStub(t, 200, `{"object":"LinkedinSearch","items":[]}`)
	h := newTestSearchHandler(pool, stub)

	cases := []struct {
		name     string
		tenantID uuid.UUID
		body     string
		status   int
		code     string
	}{
		{"no filter", withAccount.tenantID, `{}`, 400, "bad_request"},
		{"unknown size", withAccount.tenantID, `{"company_size":"12-34"}`, 400, "bad_request"},
		{"no connected account", noAccount.tenantID, `{"titles":["CEO"]}`, 409, "linkedin_not_connected"},
		{"unknown location", withAccount.tenantID, `{"titles":["CEO"],"locations":["Atlantis"]}`, 400, "unknown_location"},
		{"unknown industry", withAccount.tenantID, `{"titles":["CEO"],"industries":["Alchemy"]}`, 400, "unknown_industry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr, _ := callLinkedInSearch(t, h, tc.tenantID, tc.body)
			if rr.Code != tc.status || errorCode(t, rr) != tc.code {
				t.Errorf("status=%d code=%q want %d %s (body=%s)", rr.Code, errorCode(t, rr), tc.status, tc.code, rr.Body.String())
			}
		})
	}
	if stub.searchCalls != 0 {
		t.Errorf("search calls=%d want 0", stub.searchCalls)
	}
}

// Any other refusal (for example, no Sales Navigator on the account)
// answers 502 with Unipile's reason in words, not a raw JSON body.
func TestLinkedInSearch_UpstreamErrorIsReadable(t *testing.T) {
	pool := withMetricsPool(t)
	f := newSearchFixture(t, pool, true)
	stub := newUnipileSearchStub(t, 400, `{"status":400,"type":"errors/invalid_parameters","title":"Invalid parameters","detail":"This account has no Sales Navigator subscription."}`)
	h := newTestSearchHandler(pool, stub)

	rr, _ := callLinkedInSearch(t, h, f.tenantID, `{"titles":["Managing Partner"]}`)
	if rr.Code != http.StatusBadGateway || errorCode(t, rr) != "linkedin_search_failed" {
		t.Fatalf("status=%d code=%q want 502 linkedin_search_failed", rr.Code, errorCode(t, rr))
	}
	var body map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["message"] != "LinkedIn search failed: This account has no Sales Navigator subscription." {
		t.Errorf("message=%q", body["message"])
	}
}

// A restriction payload marks the account restricted, as a refused send
// does, and tells the user to reconnect.
func TestLinkedInSearch_RestrictedMarksAccount(t *testing.T) {
	pool := withMetricsPool(t)
	f := newSearchFixture(t, pool, true)
	stub := newUnipileSearchStub(t, 403, `{"type":"errors/account_restricted","detail":"The account is restricted"}`)
	h := newTestSearchHandler(pool, stub)

	rr, _ := callLinkedInSearch(t, h, f.tenantID, `{"titles":["Managing Partner"]}`)
	if rr.Code != http.StatusConflict || errorCode(t, rr) != "linkedin_restricted" {
		t.Fatalf("status=%d code=%q want 409 linkedin_restricted", rr.Code, errorCode(t, rr))
	}
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM linkedin_accounts WHERE id = $1`, f.accountID).Scan(&status); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if status != "restricted" {
		t.Errorf("account status=%q want restricted", status)
	}
}
