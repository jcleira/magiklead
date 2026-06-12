package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// fakeSettingsQueries lets the settings handler be exercised without a
// DB. Each field is the canned response for the matching method;
// missing tenant / wrong tenant cases override only what they need.
type fakeSettingsQueries struct {
	tenant       repository.Tenant
	tenantErr    error
	sub          repository.Subscription
	subErr       error
	gmail        []repository.GmailAccount
	gmailErr     error
	gotTenantID  pgtype.UUID
	gotSubTenant pgtype.UUID
	gotGmailTen  pgtype.UUID
}

func (f *fakeSettingsQueries) GetTenant(ctx context.Context, id pgtype.UUID) (repository.Tenant, error) {
	f.gotTenantID = id
	return f.tenant, f.tenantErr
}

func (f *fakeSettingsQueries) GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error) {
	f.gotSubTenant = tenantID
	return f.sub, f.subErr
}

func (f *fakeSettingsQueries) ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error) {
	f.gotGmailTen = tenantID
	return f.gmail, f.gmailErr
}

// TestSettings_Get_HappyJSONShape is the tracer-bullet: a request with
// a tenant in context returns the full settings payload — workspace,
// plan, usage, email_accounts — in the exact shape the frontend
// expects to consume. Subsequent tests narrow in on individual pieces.
func TestSettings_Get_HappyJSONShape(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()
	gmailID := uuid.New()

	fake := &fakeSettingsQueries{
		tenant: repository.Tenant{
			ID:   pgtype.UUID{Bytes: tenantID, Valid: true},
			Name: "Acme Workspace",
		},
		sub: repository.Subscription{
			Plan:       "growth",
			LeadsLimit: 2000,
			LeadsUsed:  pgtype.Int4{Int32: 142, Valid: true},
		},
		gmail: []repository.GmailAccount{
			{
				ID:           pgtype.UUID{Bytes: gmailID, Valid: true},
				Email:        "operator@example.com",
				TokenExpiry:  pgtype.Timestamptz{Time: now.Add(1 * time.Hour), Valid: true},
				LastPolledAt: pgtype.Timestamptz{Time: now.Add(-2 * time.Minute), Valid: true},
			},
		},
	}
	h := &SettingsHandler{queries: fake}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s want 200", rr.Code, rr.Body.String())
	}

	var got struct {
		Workspace struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"workspace"`
		Plan struct {
			Name                 string `json:"name"`
			QuotaLeadsPerMonth   int32  `json:"quota_leads_per_month"`
		} `json:"plan"`
		Usage struct {
			LeadsSavedThisPeriod int32 `json:"leads_saved_this_period"`
		} `json:"usage"`
		EmailAccounts []struct {
			ID           string  `json:"id"`
			Email        string  `json:"email"`
			Provider     string  `json:"provider"`
			Status       string  `json:"status"`
			LastPolledAt *string `json:"last_polled_at"`
		} `json:"email_accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rr.Body.String())
	}

	if got.Workspace.Name != "Acme Workspace" {
		t.Errorf("workspace.name=%q want %q", got.Workspace.Name, "Acme Workspace")
	}
	if got.Workspace.ID != tenantID.String() {
		t.Errorf("workspace.id=%q want %q", got.Workspace.ID, tenantID.String())
	}
	if got.Plan.Name != "growth" {
		t.Errorf("plan.name=%q want growth", got.Plan.Name)
	}
	if got.Plan.QuotaLeadsPerMonth != 2000 {
		t.Errorf("plan.quota_leads_per_month=%d want 2000", got.Plan.QuotaLeadsPerMonth)
	}
	if got.Usage.LeadsSavedThisPeriod != 142 {
		t.Errorf("usage.leads_saved_this_period=%d want 142", got.Usage.LeadsSavedThisPeriod)
	}
	if len(got.EmailAccounts) != 1 {
		t.Fatalf("email_accounts len=%d want 1", len(got.EmailAccounts))
	}
	acc := got.EmailAccounts[0]
	if acc.ID != gmailID.String() {
		t.Errorf("email_accounts[0].id=%q want %q", acc.ID, gmailID.String())
	}
	if acc.Email != "operator@example.com" {
		t.Errorf("email_accounts[0].email=%q", acc.Email)
	}
	if acc.Provider != "gmail" {
		t.Errorf("email_accounts[0].provider=%q want gmail", acc.Provider)
	}
	if acc.Status != "connected" {
		t.Errorf("email_accounts[0].status=%q want connected", acc.Status)
	}
	if acc.LastPolledAt == nil {
		t.Error("email_accounts[0].last_polled_at should be non-nil")
	}

	// Cross-check that all three sub-queries received the same tenantID
	// from context — a regression here would mean the handler leaked
	// data across tenants.
	wantPG := pgtype.UUID{Bytes: tenantID, Valid: true}
	if fake.gotTenantID != wantPG || fake.gotSubTenant != wantPG || fake.gotGmailTen != wantPG {
		t.Errorf("tenant ID not propagated to all queries: tenant=%v sub=%v gmail=%v",
			fake.gotTenantID, fake.gotSubTenant, fake.gotGmailTen)
	}
}

// TestSettings_Get_MissingTenant exercises the validation path: with
// no tenant in context, the handler must refuse before any query.
// EnsureTenant middleware should always run first in production, so
// reaching this branch means a routing bug — we return 401 to surface
// the misconfiguration rather than 500.
func TestSettings_Get_MissingTenant(t *testing.T) {
	h := &SettingsHandler{} // nil-queries — must not be reached
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 body=%s", rr.Code, rr.Body.String())
	}
}

// TestSettings_Get_StatusTokenExpired covers the per-mailbox indicator
// criterion: a token_expiry in the past surfaces as `token_expired`
// so the UI can show the amber dot + Reconnect CTA.
func TestSettings_Get_StatusTokenExpired(t *testing.T) {
	tenantID := uuid.New()
	now := time.Now()
	fake := &fakeSettingsQueries{
		gmail: []repository.GmailAccount{
			{
				ID:          pgtype.UUID{Bytes: uuid.New(), Valid: true},
				Email:       "expired@example.com",
				TokenExpiry: pgtype.Timestamptz{Time: now.Add(-1 * time.Hour), Valid: true},
			},
		},
	}
	h := &SettingsHandler{queries: fake}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		EmailAccounts []struct {
			Status string `json:"status"`
		} `json:"email_accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.EmailAccounts) != 1 || got.EmailAccounts[0].Status != "token_expired" {
		t.Errorf("status=%v want [token_expired]", got.EmailAccounts)
	}
}

// TestSettings_Get_NoSubscription — a tenant that somehow lacks a
// subscription row (shouldn't happen post-bootstrap, but the handler
// must degrade gracefully rather than 500). Plan falls back to "free"
// with the documented free-tier quota; usage reads 0.
func TestSettings_Get_NoSubscription(t *testing.T) {
	tenantID := uuid.New()
	fake := &fakeSettingsQueries{
		subErr: pgx.ErrNoRows,
	}
	h := &SettingsHandler{queries: fake}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req = withTenant(req, tenantID)
	rr := httptest.NewRecorder()

	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var got struct {
		Plan struct {
			Name               string `json:"name"`
			QuotaLeadsPerMonth int32  `json:"quota_leads_per_month"`
		} `json:"plan"`
		Usage struct {
			LeadsSavedThisPeriod int32 `json:"leads_saved_this_period"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Plan.Name != "free" {
		t.Errorf("plan.name=%q want free fallback", got.Plan.Name)
	}
	if got.Plan.QuotaLeadsPerMonth != 100 {
		t.Errorf("plan.quota_leads_per_month=%d want 100", got.Plan.QuotaLeadsPerMonth)
	}
	if got.Usage.LeadsSavedThisPeriod != 0 {
		t.Errorf("usage.leads_saved_this_period=%d want 0", got.Usage.LeadsSavedThisPeriod)
	}
}
