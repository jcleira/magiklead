//go:build integration

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// The Saved tab and the add-leads picker on a LinkedIn campaign render
// from GET /tenant_leads. Each saved lead must carry its LinkedIn
// profile (the URL the rail invites), title and company; a lead with no
// profile carries no linkedin_url, so the UI can mark it as one the
// LinkedIn rail cannot invite.
func TestTenantLeadList_CarriesLinkedInTitleCompany(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCampaignLeadsFixture(t, pool, "linkedin")
	profile := "https://www.linkedin.com/in/ada-" + uuid.NewString()
	withProfile := f.addPerson("Ada", "Lovelace", "Managing Partner", "Lovelace LLP", profile)
	noProfile := f.addPerson("Alan", "Turing", "Partner", "Turing LLP", "")
	f.saveLead(withProfile)
	f.saveLead(noProfile)

	h := NewTenantLeadHandler(repository.New(pool))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenant_leads", nil)
	req = withTenant(req, f.tenantID)
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Results []map[string]any `json:"results"`
		Count   int              `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	if resp.Count != 2 || len(resp.Results) != 2 {
		t.Fatalf("count=%d results=%d want 2: %s", resp.Count, len(resp.Results), rr.Body.String())
	}
	byID := map[string]map[string]any{}
	for _, row := range resp.Results {
		id, _ := row["person_id"].(string)
		byID[id] = row
	}

	ada := byID[withProfile.String()]
	if ada["linkedin_url"] != profile {
		t.Errorf("linkedin_url=%v want %q", ada["linkedin_url"], profile)
	}
	if ada["title"] != "Managing Partner" || ada["company"] != "Lovelace LLP" {
		t.Errorf("title=%v company=%v want Managing Partner / Lovelace LLP", ada["title"], ada["company"])
	}

	alan := byID[noProfile.String()]
	if _, ok := alan["linkedin_url"]; ok {
		t.Errorf("lead without a profile has linkedin_url=%v; want the key absent", alan["linkedin_url"])
	}
	if alan["title"] != "Partner" || alan["company"] != "Turing LLP" {
		t.Errorf("title=%v company=%v want Partner / Turing LLP", alan["title"], alan["company"])
	}
}
