package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/clerk"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

// fakeAccountQueries supplies the read surface AccountHandler walks.
// Every field is a canned response; tests override only what they
// need and leave the rest as zero values.
type fakeAccountQueries struct {
	tenant         repository.Tenant
	tenantErr      error
	user           repository.User
	userErr        error
	sub            repository.Subscription
	subErr         error
	campaigns      []repository.Campaign
	campaignLeads  []repository.CampaignLead
	emailEvents    []repository.EmailEvent
	tenantLeads    []repository.TenantLead
	gmailAccounts  []repository.GmailAccount
	unsubscribes   []repository.Unsubscribe
}

func (f *fakeAccountQueries) GetTenant(ctx context.Context, id pgtype.UUID) (repository.Tenant, error) {
	return f.tenant, f.tenantErr
}
func (f *fakeAccountQueries) GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error) {
	return f.user, f.userErr
}
func (f *fakeAccountQueries) GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error) {
	return f.sub, f.subErr
}
func (f *fakeAccountQueries) ListCampaigns(ctx context.Context, tenantID pgtype.UUID) ([]repository.Campaign, error) {
	return f.campaigns, nil
}
func (f *fakeAccountQueries) ListCampaignLeadsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.CampaignLead, error) {
	return f.campaignLeads, nil
}
func (f *fakeAccountQueries) ListEmailEventsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.EmailEvent, error) {
	return f.emailEvents, nil
}
func (f *fakeAccountQueries) ListTenantLeadsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.TenantLead, error) {
	return f.tenantLeads, nil
}
func (f *fakeAccountQueries) ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error) {
	return f.gmailAccounts, nil
}
func (f *fakeAccountQueries) ListUnsubscribesForTenant(ctx context.Context, tenantID pgtype.UUID) ([]repository.Unsubscribe, error) {
	return f.unsubscribes, nil
}

// fakeCascadeDeleter records every cascade step in order. Tests pull
// .calls to assert the order matches what the handler is supposed to
// walk; setting .errOn names a step that should fail, exercising the
// "AFTER Clerk delete" divergence path.
type fakeCascadeDeleter struct {
	calls []string
	errOn string
}

func (f *fakeCascadeDeleter) step(name string) error {
	f.calls = append(f.calls, name)
	if f.errOn == name {
		return errors.New("simulated " + name + " failure")
	}
	return nil
}

func (f *fakeCascadeDeleter) DeleteEmailEventsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("email_events")
}
func (f *fakeCascadeDeleter) DeleteCampaignLeadsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("campaign_leads")
}
func (f *fakeCascadeDeleter) DeleteCampaignsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("campaigns")
}
func (f *fakeCascadeDeleter) DeletePlaysByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("plays")
}
func (f *fakeCascadeDeleter) DeleteGmailAccountsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("gmail_accounts")
}
func (f *fakeCascadeDeleter) DeleteEmailAccountsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("email_accounts")
}
func (f *fakeCascadeDeleter) DeleteSubscriptionByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("subscriptions")
}
func (f *fakeCascadeDeleter) DeleteUserTenantsByTenant(ctx context.Context, tenantID pgtype.UUID) error {
	return f.step("user_tenants")
}
func (f *fakeCascadeDeleter) DeleteTenantByID(ctx context.Context, id pgtype.UUID) error {
	return f.step("tenants")
}
func (f *fakeCascadeDeleter) DeleteUserByIDIfOrphan(ctx context.Context, id pgtype.UUID) error {
	return f.step("users")
}

// fakeClerk records what was deleted; errOn != "" forces a failure
// to exercise the 502 path.
type fakeClerk struct {
	deletedClerkIDs []string
	errOn           error
}

func (f *fakeClerk) DeleteUser(ctx context.Context, clerkID string) error {
	f.deletedClerkIDs = append(f.deletedClerkIDs, clerkID)
	return f.errOn
}

// withClerkID injects the middleware's clerk-id context key so
// handlers reading it via middleware.GetUserClerkID see what the test
// supplied. Mirrors withTenant for tenant id.
func withClerkID(req *http.Request, clerkID string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), middleware.UserClerkIDKey, clerkID))
}

// runFakeTx is a txRunner replacement that just calls f synchronously
// against a fake cascadeDeleter — no real transaction. The handler
// can't tell the difference, but the test gets to inspect the call
// sequence the cascade ran in.
func runFakeTx(fake *fakeCascadeDeleter) txRunner {
	return func(ctx context.Context, f func(cascadeDeleter) error) error {
		return f(fake)
	}
}

// TestAccountExport_ContainsAllJSONFiles is the tracer-bullet: a
// signed-in tenant's export response is a real zip carrying every
// file the AC names. We don't yet assert per-table contents — that
// belongs to the file-specific tests below.
func TestAccountExport_ContainsAllJSONFiles(t *testing.T) {
	tenantID := uuid.New()
	gmailID := uuid.New()
	clerkID := "user_test_export"

	fq := &fakeAccountQueries{
		tenant: repository.Tenant{
			ID:   pgtype.UUID{Bytes: tenantID, Valid: true},
			Name: "Export Workspace",
		},
		user: repository.User{
			ID:      pgtype.UUID{Bytes: uuid.New(), Valid: true},
			ClerkID: clerkID,
			Email:   "owner@example.com",
		},
		sub: repository.Subscription{
			Plan:       "growth",
			LeadsLimit: 2000,
		},
		gmailAccounts: []repository.GmailAccount{
			{
				ID:           pgtype.UUID{Bytes: gmailID, Valid: true},
				Email:        "sender@example.com",
				AccessToken:  "secret-access-token",
				RefreshToken: "secret-refresh-token",
			},
		},
	}
	h := &AccountHandler{queries: fq}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/export", nil)
	req = withTenant(req, tenantID)
	req = withClerkID(req, clerkID)
	rr := httptest.NewRecorder()

	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type=%q want application/zip", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if cd == "" || cd[:len("attachment;")] != "attachment;" {
		t.Errorf("Content-Disposition=%q want attachment with filename", cd)
	}

	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("response body is not a valid zip: %v", err)
	}

	wantFiles := []string{
		"tenant.json",
		"user.json",
		"subscription.json",
		"campaigns.json",
		"sequences.json",
		"campaign_leads.json",
		"email_events.json",
		"tenant_leads.json",
		"gmail_accounts.json",
		"unsubscribes.json",
	}
	gotFiles := make([]string, len(zr.File))
	for i, f := range zr.File {
		gotFiles[i] = f.Name
	}
	sort.Strings(wantFiles)
	sort.Strings(gotFiles)
	if !equalStringSlices(gotFiles, wantFiles) {
		t.Fatalf("zip files=%v want %v", gotFiles, wantFiles)
	}

	// Every file must parse as JSON — a broken Encode would otherwise
	// only fail at the user's machine.
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		var any any
		if err := json.Unmarshal(data, &any); err != nil {
			t.Errorf("%s is not valid JSON: %v\ncontent=%s", f.Name, err, string(data))
		}
	}

	// Gmail tokens must be redacted — shipping them would let anyone
	// with the zip impersonate the user's mailbox.
	for _, f := range zr.File {
		if f.Name != "gmail_accounts.json" {
			continue
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if bytes.Contains(data, []byte("secret-access-token")) ||
			bytes.Contains(data, []byte("secret-refresh-token")) {
			t.Errorf("gmail_accounts.json leaked OAuth tokens: %s", data)
		}
	}
}

// TestAccountExport_MissingTenant — the handler must refuse with 401
// when EnsureTenant didn't populate the context (routing bug). The
// nil-queries handler proves the validation path doesn't reach any
// DB call.
func TestAccountExport_MissingTenant(t *testing.T) {
	h := &AccountHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/export", nil)
	rr := httptest.NewRecorder()
	h.Export(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rr.Code)
	}
}

// TestAccountExport_MissingClerkID — the export needs a Clerk identity
// to write user.json. No clerk_id in context → 401, no DB calls.
func TestAccountExport_MissingClerkID(t *testing.T) {
	h := &AccountHandler{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/export", nil)
	req = withTenant(req, uuid.New())
	rr := httptest.NewRecorder()
	h.Export(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rr.Code)
	}
}

// TestAccountDelete_HappyPath — Clerk gets called first, then every
// cascade step fires in the documented order, and the response is
// 204 No Content.
func TestAccountDelete_HappyPath(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	clerkID := "user_test_delete"

	fq := &fakeAccountQueries{
		user: repository.User{
			ID:      pgtype.UUID{Bytes: userID, Valid: true},
			ClerkID: clerkID,
		},
	}
	fcd := &fakeCascadeDeleter{}
	fc := &fakeClerk{}

	h := &AccountHandler{queries: fq, tx: runFakeTx(fcd), clerk: fc}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil)
	req = withTenant(req, tenantID)
	req = withClerkID(req, clerkID)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if len(fc.deletedClerkIDs) != 1 || fc.deletedClerkIDs[0] != clerkID {
		t.Errorf("clerk deleted=%v want [%s]", fc.deletedClerkIDs, clerkID)
	}
	wantOrder := []string{
		"email_events", "campaign_leads", "campaigns", "plays",
		"gmail_accounts", "email_accounts", "subscriptions",
		"user_tenants", "tenants", "users",
	}
	if !equalStringSlices(fcd.calls, wantOrder) {
		t.Errorf("cascade order=%v want %v", fcd.calls, wantOrder)
	}
}

// TestAccountDelete_ClerkFails — when Clerk returns an error, the
// local cascade must not run. The handler returns 502 so the client
// knows to retry; the local data is untouched and ready for the next
// attempt.
func TestAccountDelete_ClerkFails(t *testing.T) {
	tenantID := uuid.New()
	clerkID := "user_test_clerk_fail"

	fq := &fakeAccountQueries{
		user: repository.User{
			ID:      pgtype.UUID{Bytes: uuid.New(), Valid: true},
			ClerkID: clerkID,
		},
	}
	fcd := &fakeCascadeDeleter{}
	fc := &fakeClerk{errOn: errors.New("clerk returned 500")}

	h := &AccountHandler{queries: fq, tx: runFakeTx(fcd), clerk: fc}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil)
	req = withTenant(req, tenantID)
	req = withClerkID(req, clerkID)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want 502 body=%s", rr.Code, rr.Body.String())
	}
	if len(fcd.calls) != 0 {
		t.Errorf("cascade ran despite Clerk failure: %v", fcd.calls)
	}
}

// TestAccountDelete_ClerkNotFound — Clerk reports the user is already
// gone. That's success from our point of view; the local cascade still
// runs, and we return 204.
func TestAccountDelete_ClerkNotFound(t *testing.T) {
	tenantID := uuid.New()
	clerkID := "user_test_clerk_404"

	fq := &fakeAccountQueries{
		user: repository.User{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, ClerkID: clerkID},
	}
	fcd := &fakeCascadeDeleter{}
	fc := &fakeClerk{errOn: clerk.ErrNotFound}

	h := &AccountHandler{queries: fq, tx: runFakeTx(fcd), clerk: fc}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil)
	req = withTenant(req, tenantID)
	req = withClerkID(req, clerkID)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 body=%s", rr.Code, rr.Body.String())
	}
	if len(fcd.calls) == 0 {
		t.Error("cascade did not run despite ErrNotFound being treated as success")
	}
}

// TestAccountDelete_AlreadyDeleted — no local user row for this clerk
// id (already cleaned up). Handler returns 204 without calling Clerk
// or DB cascade: idempotency for retried delete requests.
func TestAccountDelete_AlreadyDeleted(t *testing.T) {
	tenantID := uuid.New()
	clerkID := "user_test_already_deleted"

	fq := &fakeAccountQueries{userErr: pgx.ErrNoRows}
	fcd := &fakeCascadeDeleter{}
	fc := &fakeClerk{}

	h := &AccountHandler{queries: fq, tx: runFakeTx(fcd), clerk: fc}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", nil)
	req = withTenant(req, tenantID)
	req = withClerkID(req, clerkID)
	rr := httptest.NewRecorder()

	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d want 204 (idempotent already-deleted) body=%s", rr.Code, rr.Body.String())
	}
	if len(fc.deletedClerkIDs) != 0 {
		t.Errorf("clerk called for already-deleted user: %v", fc.deletedClerkIDs)
	}
	if len(fcd.calls) != 0 {
		t.Errorf("cascade ran for already-deleted user: %v", fcd.calls)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
