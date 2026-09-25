//go:build integration

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/handler"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// seedSearchPerson stores a person with a current job at a new
// organization, a location, an updated_at in the past (ageDays), and a
// linkedin_url identifier when linkedinURL is not empty. Rows are torn
// down with the test.
func seedSearchPerson(t *testing.T, pool *pgxpool.Pool, title, location, linkedinURL string, ageDays int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	personID := uuid.New()
	orgID := uuid.New()
	name := "Search Person " + personID.String()[:8]
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	exec(`INSERT INTO organizations (id, canonical_name) VALUES ($1, $2)`, orgID, "Search Org "+orgID.String()[:8])
	exec(`INSERT INTO persons (id, canonical_name, normalized_name, location, updated_at)
	      VALUES ($1, $2, $3, $4, NOW() - make_interval(days => $5))`,
		personID, name, name, location, ageDays)
	exec(`INSERT INTO employments (person_id, organization_id, title, is_current) VALUES ($1, $2, $3, TRUE)`,
		personID, orgID, title)
	if linkedinURL != "" {
		exec(`INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) VALUES ($1, 'linkedin_url', $2, TRUE)`,
			personID, linkedinURL)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM persons WHERE id = $1`, personID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return personID
}

type searchResultRow struct {
	PersonID    string  `json:"person_id"`
	LinkedInURL *string `json:"linkedin_url"`
}

func runLeadSearch(t *testing.T, h *handler.LeadSearchHandler, tenantID uuid.UUID, body map[string]any) []searchResultRow {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/leads/search", bytes.NewReader(mustJSON(t, body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, tenantID))
	rr := httptest.NewRecorder()
	h.Search(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Results []searchResultRow `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	return resp.Results
}

// "With LinkedIn profile" keeps only people the LinkedIn rail can
// invite; off, the same search returns both people.
func TestLeadSearch_WithLinkedInKeepsOnlyInvitablePeople(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)
	title := "Chief Llama Wrangler " + uuid.NewString()[:8]
	profile := "https://www.linkedin.com/in/llama-" + uuid.NewString()
	withProfile := seedSearchPerson(t, pool, title, "Denver, Colorado, United States", profile, 0)
	seedSearchPerson(t, pool, title, "Denver, Colorado, United States", "", 0)

	h := handler.NewLeadSearchHandler(repository.New(pool), nil)

	got := runLeadSearch(t, h, tenantID, map[string]any{"titles": []string{title}, "with_linkedin": true, "limit": 25})
	if len(got) != 1 || got[0].PersonID != withProfile.String() {
		t.Fatalf("with_linkedin results=%+v want only %s", got, withProfile)
	}
	if got[0].LinkedInURL == nil || *got[0].LinkedInURL != profile {
		t.Errorf("linkedin_url=%v want %q", got[0].LinkedInURL, profile)
	}

	all := runLeadSearch(t, h, tenantID, map[string]any{"titles": []string{title}, "limit": 25})
	if len(all) != 2 {
		t.Errorf("without with_linkedin results=%d want 2", len(all))
	}
}

// Without PDL nothing refreshes a row, so a location filter must not
// hide a person whose row is older than the 90-day window.
func TestLeadSearch_NoPDLShowsOlderRows(t *testing.T) {
	pool := withPool(t)
	tenantID := freshTenant(t, pool)
	suffix := uuid.NewString()[:8]
	title := "Head of Llamas " + suffix
	place := "Llamaville " + suffix
	personID := seedSearchPerson(t, pool, title, place+", United States", "", 200)

	h := handler.NewLeadSearchHandler(repository.New(pool), nil)
	got := runLeadSearch(t, h, tenantID, map[string]any{
		"titles":    []string{title},
		"locations": []string{place},
		"limit":     25,
	})
	if len(got) != 1 || got[0].PersonID != personID.String() {
		t.Errorf("results=%+v want the 200-day-old person %s", got, personID)
	}
}
