//go:build integration

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Run with:
//   DATABASE_URL=… go test -tags=integration ./internal/handler/...
//
// Covers the campaign-leads routes the LinkedIn campaign page binds to:
// the leads list shows the profile the invite worker will use, adding
// saved leads to a LinkedIn campaign keeps out people it can never
// invite, and every campaign route is scoped to the caller's tenant.

// campaignLeadsFixture is a tenant + user + play + one campaign on the
// given channel. People, saved leads and campaign leads are added per
// test and torn down with the base rows.
type campaignLeadsFixture struct {
	t          *testing.T
	pool       *pgxpool.Pool
	tenantID   uuid.UUID
	userID     uuid.UUID
	playID     uuid.UUID
	campaignID uuid.UUID

	personIDs []uuid.UUID
	orgIDs    []uuid.UUID
	legacyIDs []uuid.UUID
}

func newCampaignLeadsFixture(t *testing.T, pool *pgxpool.Pool, channel string) *campaignLeadsFixture {
	t.Helper()
	f := &campaignLeadsFixture{
		t:          t,
		pool:       pool,
		tenantID:   uuid.New(),
		userID:     uuid.New(),
		playID:     uuid.New(),
		campaignID: uuid.New(),
	}
	run := uuid.NewString()
	metricsExec(t, pool, `INSERT INTO users (id, clerk_id, email) VALUES ($1, $2, $3)`,
		f.userID, "user_campaign_leads_"+run, "owner-"+run+"@example.com")
	metricsExec(t, pool, `INSERT INTO tenants (id, name) VALUES ($1, $2)`,
		f.tenantID, "Campaign Leads Test "+run)
	metricsExec(t, pool, `INSERT INTO user_tenants (user_id, tenant_id, role) VALUES ($1, $2, 'owner')`,
		f.userID, f.tenantID)
	metricsExec(t, pool, `INSERT INTO plays (id, tenant_id, name, icp, search_query, channels) VALUES ($1, $2, $3, '{}'::jsonb, '{}'::jsonb, ARRAY[$4]::text[])`,
		f.playID, f.tenantID, "Campaign Leads Play", channel)
	metricsExec(t, pool, `INSERT INTO campaigns (id, tenant_id, play_id, name, status, channel, sequence) VALUES ($1, $2, $3, $4, 'draft', $5, '[]'::jsonb)`,
		f.campaignID, f.tenantID, f.playID, "Campaign Leads Campaign", channel)

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `DELETE FROM campaign_leads WHERE campaign_id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM campaigns WHERE id = $1`, f.campaignID)
		_, _ = pool.Exec(ctx, `DELETE FROM plays WHERE id = $1`, f.playID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenant_leads WHERE tenant_id = $1`, f.tenantID)
		for _, id := range f.legacyIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM leads WHERE id = $1`, id)
		}
		for _, id := range f.personIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM persons WHERE id = $1`, id) // cascades identifiers + employments
		}
		for _, id := range f.orgIDs {
			_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, id)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM user_tenants WHERE tenant_id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, f.tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, f.userID)
	})
	return f
}

// addPerson stores a person with a current job, and with a linkedin_url
// identifier when linkedinURL is not empty.
func (f *campaignLeadsFixture) addPerson(first, last, title, company, linkedinURL string) uuid.UUID {
	f.t.Helper()
	personID := uuid.New()
	orgID := uuid.New()
	name := first + " " + last
	metricsExec(f.t, f.pool, `INSERT INTO persons (id, canonical_name, normalized_name, first_name, last_name) VALUES ($1, $2, $3, $4, $5)`,
		personID, name, strings.ToLower(name)+" "+personID.String(), first, last)
	metricsExec(f.t, f.pool, `INSERT INTO organizations (id, canonical_name) VALUES ($1, $2)`, orgID, company)
	metricsExec(f.t, f.pool, `INSERT INTO employments (person_id, organization_id, title, is_current) VALUES ($1, $2, $3, TRUE)`,
		personID, orgID, title)
	if linkedinURL != "" {
		metricsExec(f.t, f.pool, `INSERT INTO person_identifiers (person_id, identifier_type, identifier_value, is_primary) VALUES ($1, 'linkedin_url', $2, TRUE)`,
			personID, linkedinURL)
	}
	f.personIDs = append(f.personIDs, personID)
	f.orgIDs = append(f.orgIDs, orgID)
	return personID
}

func (f *campaignLeadsFixture) saveLead(personID uuid.UUID) {
	f.t.Helper()
	metricsExec(f.t, f.pool, `INSERT INTO tenant_leads (tenant_id, person_id, added_by_user_id) VALUES ($1, $2, $3)`,
		f.tenantID, personID, f.userID)
}

func (f *campaignLeadsFixture) campaignStatus() string {
	f.t.Helper()
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM campaigns WHERE id = $1`, f.campaignID).Scan(&status); err != nil {
		f.t.Fatalf("scan campaign status: %v", err)
	}
	return status
}

func callCampaignRoute(t *testing.T, route http.HandlerFunc, tenantID, campaignID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/campaigns", strings.NewReader(body))
	req = withTenant(req, tenantID)
	req = withURLParams(req, map[string]string{"id": campaignID.String()})
	rr := httptest.NewRecorder()
	route(rr, req)
	return rr
}

// A LinkedIn lead is added by person_id (AddPersonToCampaign), so it has
// no legacy leads row. The list must still show the person's LinkedIn
// profile — the same identifier the invite worker sends to — and a
// legacy lead keeps its own column.
func TestListLeads_ShowsPersonLinkedInURL(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCampaignLeadsFixture(t, pool, "linkedin")
	profile := "https://www.linkedin.com/in/grace-" + uuid.NewString()
	personID := f.addPerson("Grace", "Hopper", "Managing Partner", "Hopper & Co LLP", profile)
	metricsExec(t, pool, `INSERT INTO campaign_leads (campaign_id, person_id, status) VALUES ($1, $2, 'queued')`,
		f.campaignID, personID)

	legacyID := uuid.New()
	legacyProfile := "https://www.linkedin.com/in/legacy-" + uuid.NewString()
	metricsExec(t, pool, `INSERT INTO leads (id, first_name, last_name, linkedin_url) VALUES ($1, 'Lena', 'Legacy', $2)`,
		legacyID, legacyProfile)
	f.legacyIDs = append(f.legacyIDs, legacyID)
	metricsExec(t, pool, `INSERT INTO campaign_leads (campaign_id, lead_id, status) VALUES ($1, $2, 'queued')`,
		f.campaignID, legacyID)

	h := NewCampaignHandler(repository.New(pool), nil)
	req := httptest.NewRequest(http.MethodGet, "/campaigns/leads", nil)
	req = withTenant(req, f.tenantID)
	req = withURLParams(req, map[string]string{"id": f.campaignID.String()})
	rr := httptest.NewRecorder()
	h.ListLeads(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var rows []struct {
		PersonID    *string `json:"person_id"`
		FirstName   string  `json:"first_name"`
		Title       *string `json:"title"`
		Company     string  `json:"company"`
		LinkedinURL *string `json:"linkedin_url"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2: %s", len(rows), rr.Body.String())
	}
	byName := map[string]int{}
	for i, row := range rows {
		byName[row.FirstName] = i
	}
	grace := rows[byName["Grace"]]
	if grace.LinkedinURL == nil || *grace.LinkedinURL != profile {
		t.Errorf("person lead linkedin_url=%v want %q", grace.LinkedinURL, profile)
	}
	if grace.PersonID == nil || *grace.PersonID != personID.String() {
		t.Errorf("person lead person_id=%v want %s", grace.PersonID, personID)
	}
	if grace.Title == nil || *grace.Title != "Managing Partner" || grace.Company != "Hopper & Co LLP" {
		t.Errorf("person lead title=%v company=%q", grace.Title, grace.Company)
	}
	lena := rows[byName["Lena"]]
	if lena.LinkedinURL == nil || *lena.LinkedinURL != legacyProfile {
		t.Errorf("legacy lead linkedin_url=%v want %q", lena.LinkedinURL, legacyProfile)
	}
}

// A LinkedIn campaign accepts only saved people with a LinkedIn profile:
// the invite worker cannot send to anyone else. The response says why
// each person was skipped, and a person already in the campaign is not
// counted as added a second time.
func TestAddLeads_LinkedInCampaignSkipsPersonWithoutProfile(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCampaignLeadsFixture(t, pool, "linkedin")
	withProfile := f.addPerson("Ada", "Lovelace", "Managing Partner", "Lovelace LLP",
		"https://www.linkedin.com/in/ada-"+uuid.NewString())
	noProfile := f.addPerson("Alan", "Turing", "Partner", "Turing LLP", "")
	notSaved := f.addPerson("Karen", "Jones", "Partner", "Jones LLP",
		"https://www.linkedin.com/in/karen-"+uuid.NewString())
	f.saveLead(withProfile)
	f.saveLead(noProfile)

	h := NewCampaignHandler(repository.New(pool), nil)
	body := `{"person_ids":["` + withProfile.String() + `","` + noProfile.String() + `","` + notSaved.String() + `","not-a-uuid"]}`
	rr := callCampaignRoute(t, h.AddLeads, f.tenantID, f.campaignID, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	if resp["added"] != 1 || resp["skipped"] != 3 || resp["skipped_no_linkedin"] != 1 {
		t.Errorf("response=%v want added=1 skipped=3 skipped_no_linkedin=1", resp)
	}

	rows, err := pool.Query(context.Background(),
		`SELECT person_id, status FROM campaign_leads WHERE campaign_id = $1`, f.campaignID)
	if err != nil {
		t.Fatalf("query campaign_leads: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var personID uuid.UUID
		var status string
		if err := rows.Scan(&personID, &status); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, personID.String()+":"+status)
	}
	if len(got) != 1 || got[0] != withProfile.String()+":queued" {
		t.Errorf("campaign_leads=%v want only %s:queued", got, withProfile)
	}

	// Adding the same person again inserts nothing and says so.
	rr = callCampaignRoute(t, h.AddLeads, f.tenantID, f.campaignID, `{"person_ids":["`+withProfile.String()+`"]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("re-add status=%d body=%s", rr.Code, rr.Body.String())
	}
	resp = map[string]int{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	if resp["added"] != 0 || resp["skipped"] != 1 {
		t.Errorf("re-add response=%v want added=0 skipped=1", resp)
	}
}

// The LinkedIn-profile rule is for the LinkedIn rail only: an email
// campaign still takes a saved person who has no LinkedIn profile.
func TestAddLeads_EmailCampaignTakesPersonWithoutProfile(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCampaignLeadsFixture(t, pool, "email")
	noProfile := f.addPerson("Alan", "Turing", "Partner", "Turing LLP", "")
	f.saveLead(noProfile)

	h := NewCampaignHandler(repository.New(pool), nil)
	rr := callCampaignRoute(t, h.AddLeads, f.tenantID, f.campaignID, `{"person_ids":["`+noProfile.String()+`"]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]int
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	if resp["added"] != 1 || resp["skipped"] != 0 {
		t.Errorf("response=%v want added=1 skipped=0", resp)
	}
}

// Start, Pause and the leads list must not act on another tenant's
// campaign: the status update is keyed on the campaign id alone, so the
// route has to check the owner first. 404 matches a missing campaign.
func TestCampaignRoutes_RejectOtherTenant(t *testing.T) {
	pool := withMetricsPool(t)
	f := newCampaignLeadsFixture(t, pool, "linkedin")
	h := NewCampaignHandler(repository.New(pool), nil)
	other := uuid.New()

	for name, route := range map[string]http.HandlerFunc{
		"start":     h.Start,
		"pause":     h.Pause,
		"listLeads": h.ListLeads,
	} {
		t.Run(name, func(t *testing.T) {
			rr := callCampaignRoute(t, route, other, f.campaignID, "")
			if rr.Code != http.StatusNotFound {
				t.Errorf("status=%d want 404 body=%s", rr.Code, rr.Body.String())
			}
			if got := f.campaignStatus(); got != "draft" {
				t.Errorf("campaign status=%q want draft (untouched)", got)
			}
		})
	}

	// The owner still can.
	rr := callCampaignRoute(t, h.Start, f.tenantID, f.campaignID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("owner start status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := f.campaignStatus(); got != "active" {
		t.Errorf("campaign status=%q want active after the owner starts it", got)
	}
}
