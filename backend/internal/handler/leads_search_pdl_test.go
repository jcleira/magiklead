//go:build integration

package handler_test

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

	"github.com/jcleira/magiklead/backend/internal/handler"
	"github.com/jcleira/magiklead/backend/internal/leads/pdl"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Run with:
//   devpods exec api go test -tags=integration ./internal/handler/...

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

func freshTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		id, "Search Test "+id.String())
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return id
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

// Issue #7 AC: industries, company_size, locations no longer 501;
// canonical miss with PDL filters triggers a PDL call; results write
// through to the canonical and are returned in the response.
func TestLeadSearch_PDLFallback_OnCanonicalMiss(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)
	suffix := "-" + uuid.NewString()[:8]

	industry := "handler-fallback-industry" + suffix
	title := "VP Marketing " + suffix
	body := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{
			{
				"id":                   "pdl-handler" + suffix,
				"full_name":            "Hannah Handler" + suffix,
				"first_name":           "Hannah",
				"last_name":            "Handler",
				"job_title":            title,
				"job_company_name":     "Handler Co",
				"job_company_website":  "handlerco-" + strings.TrimPrefix(suffix, "-") + ".test",
				"job_company_industry": industry,
				"job_company_size":     "51-200",
				"location_name":        "Berlin, Germany",
				"work_email":           "hannah" + suffix + "@handlerco.test",
			},
		},
	})
	stub := newStub(t, 200, body)

	pdlModule := pdl.New(pool, "dev", stub.Client())
	pdlModule.SetBaseURL(stub.URL)

	h := handler.NewLeadSearchHandler(repository.New(pool), pdlModule)

	reqBody := mustJSON(t, map[string]any{
		"industries":   []string{industry},
		"company_size": "51-200",
		"locations":    []string{"Berlin"},
		"description":  "Marketing leaders at European SaaS",
		"limit":        25,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search",
		bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.calls != 1 {
		t.Errorf("PDL hits=%d, want 1 (canonical was empty)", stub.calls)
	}

	var resp struct {
		Results []struct {
			Name             string   `json:"name"`
			Email            string   `json:"email"`
			EmailVerified    bool     `json:"email_verified"`
			OrganizationName string   `json:"organization_name"`
			Industries       []string `json:"industries"`
			CompanySize      string   `json:"company_size"`
			Location         string   `json:"location"`
		} `json:"results"`
		Count     int  `json:"count"`
		PDLCalled bool `json:"pdl_called"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if !resp.PDLCalled {
		t.Errorf("pdl_called=false; expected true on canonical miss with PDL filters")
	}
	if resp.Count < 1 {
		t.Fatalf("results count=%d, want >=1", resp.Count)
	}
	r := resp.Results[0]
	if r.Name != "Hannah Handler"+suffix {
		t.Errorf("Name=%q", r.Name)
	}
	if r.Email != "hannah"+suffix+"@handlerco.test" {
		t.Errorf("Email=%q", r.Email)
	}
	if !r.EmailVerified {
		t.Errorf("EmailVerified should be true (pdl-verified)")
	}
	if r.CompanySize != "51-200" {
		t.Errorf("CompanySize=%q", r.CompanySize)
	}
}

// AC: when only legacy filters (titles) are present, PDL is not
// touched even if the canonical is empty — the integration is opt-in
// via the new filter dimensions / description.
func TestLeadSearch_PDLNotCalled_OnTitlesOnlyMiss(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)

	stub := newStub(t, 200, []byte(`{"data":[]}`))
	pdlModule := pdl.New(pool, "dev", stub.Client())
	pdlModule.SetBaseURL(stub.URL)

	h := handler.NewLeadSearchHandler(repository.New(pool), pdlModule)

	reqBody := mustJSON(t, map[string]any{
		"titles": []string{"NonExistentTitle-" + uuid.NewString()},
		"limit":  10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search",
		bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if stub.calls != 0 {
		t.Errorf("PDL hits=%d, want 0 (titles-only search must stay canonical-only)", stub.calls)
	}
}

// AC: PDL_API_KEY missing in dev → handler tolerates and serves
// canonical-only without panic or 500.
func TestLeadSearch_NoPDLConfigured(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)

	// Nil module is the realistic dev shape; nil-safe via the
	// handler's `h.pdl != nil` gate.
	h := handler.NewLeadSearchHandler(repository.New(pool), nil)

	reqBody := mustJSON(t, map[string]any{
		"industries": []string{"no-pdl-industry-" + uuid.NewString()[:8]},
		"limit":      10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search",
		bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		PDLCalled bool `json:"pdl_called"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.PDLCalled {
		t.Errorf("pdl_called=true with nil module")
	}
}

// TestLeadSearch_PDLFallback_OnShadowedEmailless covers the has_email
// cache-shadow guard. A canonical person that matches the ICP and is
// flagged has_email=TRUE but has NO actual address (exactly what a
// prior free-tier PDL write-through leaves behind) must NOT shadow the
// PDL fetch when the caller asked for emails — PDL is still called and
// the real address resolved. Without the guard, the fresh emailless row
// satisfies with_email and PDL is skipped (pdl_called=false).
func TestLeadSearch_PDLFallback_OnShadowedEmailless(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)
	suffix := "-" + uuid.NewString()[:8]
	ctx := context.Background()

	industry := "shadow-industry" + suffix
	title := "Growth Lead " + suffix
	domain := "shadowco" + strings.TrimPrefix(suffix, "-") + ".test"

	// Seed an "emailable but address-less" canonical match.
	orgID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO organizations (id, canonical_name, primary_domain, industries, size_range)
		 VALUES ($1,$2,$3,$4,$5)`,
		orgID, "Shadow Co"+suffix, domain, []string{industry}, "51-200"); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	personID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO persons (id, canonical_name, normalized_name, has_email, updated_at)
		 VALUES ($1,$2,$3,TRUE,NOW())`,
		personID, "Sammy Shadow"+suffix, "sammy shadow"+suffix); err != nil {
		t.Fatalf("seed person: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO employments (person_id, organization_id, title, is_current)
		 VALUES ($1,$2,$3,TRUE)`,
		personID, orgID, title); err != nil {
		t.Fatalf("seed employment: %v", err)
	}

	// PDL stub returns a matching record WITH a real work_email.
	body := mustJSON(t, map[string]any{
		"status": 200,
		"data": []map[string]any{{
			"id":                   "pdl-shadow" + suffix,
			"full_name":            "Sammy Shadow" + suffix,
			"job_title":            title,
			"job_company_name":     "Shadow Co" + suffix,
			"job_company_website":  domain,
			"job_company_industry": industry,
			"job_company_size":     "51-200",
			"work_email":           "sammy" + suffix + "@shadowco.test",
		}},
	})
	stub := newStub(t, 200, body)
	pdlModule := pdl.New(pool, "dev", stub.Client())
	pdlModule.SetBaseURL(stub.URL)
	h := handler.NewLeadSearchHandler(repository.New(pool), pdlModule)

	reqBody := mustJSON(t, map[string]any{
		"industries":   []string{industry},
		"company_size": "51-200",
		"with_email":   true,
		"limit":        25,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	rr := httptest.NewRecorder()

	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	// The guard must fire: an emailless has_email row must not shadow
	// the PDL fetch.
	if stub.calls != 1 {
		t.Errorf("PDL hits=%d, want 1 (emailless has_email row must not shadow the fetch)", stub.calls)
	}
	var resp struct {
		Results []struct {
			Email    string `json:"email"`
			HasEmail bool   `json:"has_email"`
		} `json:"results"`
		PDLCalled bool `json:"pdl_called"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}
	if !resp.PDLCalled {
		t.Error("pdl_called=false; the shadow guard should have triggered a PDL fetch")
	}
	found := false
	for _, r := range resp.Results {
		if r.Email == "sammy"+suffix+"@shadowco.test" {
			found = true
		}
	}
	if !found {
		t.Errorf("real address not resolved after PDL fallthrough; results=%+v", resp.Results)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	buf, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return buf
}
