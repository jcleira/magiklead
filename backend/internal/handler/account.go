package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/clerk"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// accountQueries is the slice of repository.Queries the account
// handler needs. Listed explicitly so test fakes can implement a
// stable surface; the production handler is wired with the real
// *repository.Queries.
type accountQueries interface {
	GetTenant(ctx context.Context, id pgtype.UUID) (repository.Tenant, error)
	GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error)
	GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error)
	ListCampaigns(ctx context.Context, tenantID pgtype.UUID) ([]repository.Campaign, error)
	ListCampaignLeadsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.CampaignLead, error)
	ListEmailEventsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.EmailEvent, error)
	ListTenantLeadsForExport(ctx context.Context, tenantID pgtype.UUID) ([]repository.TenantLead, error)
	ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error)
	ListUnsubscribesForTenant(ctx context.Context, tenantID pgtype.UUID) ([]repository.Unsubscribe, error)
}

// cascadeDeleter is the subset of repository.Queries the delete tx
// needs. Same WithTx contract as the rest of the codebase: the tx
// wrapper exposes the same method set as *Queries.
type cascadeDeleter interface {
	DeleteEmailEventsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteCampaignLeadsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteCampaignsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeletePlaysByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteGmailAccountsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteEmailAccountsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteSubscriptionByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteUserTenantsByTenant(ctx context.Context, tenantID pgtype.UUID) error
	DeleteTenantByID(ctx context.Context, id pgtype.UUID) error
	DeleteUserByIDIfOrphan(ctx context.Context, id pgtype.UUID) error
}

// txRunner wraps the "open tx, run f against a cascadeDeleter, commit"
// pattern. Production wires it to pgx.BeginFunc against the shared
// pool; tests substitute an in-memory runner that calls f against a
// fake cascadeDeleter and records the call sequence.
type txRunner func(ctx context.Context, f func(cascadeDeleter) error) error

// clerkDeleter is the subset of internal/clerk.Client AccountHandler
// uses. Lets tests inject a stub that records (and optionally fails)
// the user-delete call without standing up a real HTTP client.
type clerkDeleter interface {
	DeleteUser(ctx context.Context, clerkID string) error
}

// AccountHandler serves /api/v1/account — the self-service export +
// delete endpoints (issue #11). Both routes are tenant-scoped; the
// caller's Clerk identity flows through middleware as usual.
type AccountHandler struct {
	queries accountQueries
	tx      txRunner
	clerk   clerkDeleter
}

// NewAccountHandler wires the handler against the live DB pool and a
// real Clerk Backend API client. If clerkSecret is empty, the delete
// endpoint still works against local DB (rare in tests / dev) but
// production paths must always set it.
func NewAccountHandler(q *repository.Queries, pool *pgxpool.Pool, clerkSecret string) *AccountHandler {
	tx := txRunner(func(ctx context.Context, f func(cascadeDeleter) error) error {
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			return f(q.WithTx(tx))
		})
	})
	return &AccountHandler{
		queries: q,
		tx:      tx,
		clerk:   clerk.New(clerkSecret),
	}
}

// exportPayload is the in-memory accumulator for an export. Each
// field becomes one JSON file in the resulting zip. Marshalled in
// one Encode pass per file so we don't hold three copies of every
// row at the same time.
type exportPayload struct {
	Tenant        any   `json:"-"`
	User          any   `json:"-"`
	Subscription  any   `json:"-"`
	Campaigns     any   `json:"-"`
	Sequences     any   `json:"-"`
	CampaignLeads any   `json:"-"`
	EmailEvents   any   `json:"-"`
	TenantLeads   any   `json:"-"`
	GmailAccounts any   `json:"-"`
	Unsubscribes  any   `json:"-"`
}

// Export handles POST /api/v1/account/export. Synchronous: collects
// all tenant-scoped rows and streams a zip back. For v1 this is fine
// — the largest single export the validation window will see is the
// operator's own ~100-lead campaigns. Async + presigned URL handoff
// is a follow-up if exports grow.
func (h *AccountHandler) Export(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	if tenantID == uuid.Nil {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	clerkID := middleware.GetUserClerkID(r.Context())
	if clerkID == "" {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	pgTenant := pgUUID(tenantID)
	ctx := r.Context()

	tenant, err := h.queries.GetTenant(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "tenant lookup: " + err.Error()})
		return
	}
	user, err := h.queries.GetUserByClerkID(ctx, clerkID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "user lookup: " + err.Error()})
		return
	}
	sub, subErr := h.queries.GetSubscription(ctx, pgTenant)
	// Missing subscription row is not fatal — bootstrap may not have
	// run yet for very-fresh tenants. Emit a null entry.
	var subAny any
	if subErr == nil {
		subAny = sub
	} else if !errors.Is(subErr, pgx.ErrNoRows) {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "subscription lookup: " + subErr.Error()})
		return
	}

	campaigns, err := h.queries.ListCampaigns(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "campaigns: " + err.Error()})
		return
	}
	campaignLeads, err := h.queries.ListCampaignLeadsForExport(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "campaign_leads: " + err.Error()})
		return
	}
	emailEvents, err := h.queries.ListEmailEventsForExport(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "email_events: " + err.Error()})
		return
	}
	tenantLeads, err := h.queries.ListTenantLeadsForExport(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "tenant_leads: " + err.Error()})
		return
	}
	gmailAccounts, err := h.queries.ListGmailAccounts(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "gmail_accounts: " + err.Error()})
		return
	}
	unsubs, err := h.queries.ListUnsubscribesForTenant(ctx, pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "unsubscribes: " + err.Error()})
		return
	}

	// Sequences come from the .sequence JSONB on each campaign — they
	// aren't a standalone table. Project a small {campaign_id, name,
	// sequence} per row so the export carries the data in the form
	// the AC names.
	sequences := make([]map[string]any, 0, len(campaigns))
	for _, c := range campaigns {
		sequences = append(sequences, map[string]any{
			"campaign_id": c.ID,
			"name":        c.Name,
			"sequence":    c.Sequence,
		})
	}

	// Token fields on Gmail rows are redacted before they leave the
	// process. The export goes straight to the user, so we cannot ship
	// the actual OAuth tokens — they would let anyone with the zip
	// impersonate the user's Gmail.
	redactedGmail := make([]repository.GmailAccount, len(gmailAccounts))
	for i, g := range gmailAccounts {
		g.AccessToken = ""
		g.RefreshToken = ""
		redactedGmail[i] = g
	}

	files := []struct {
		name string
		data any
	}{
		{"tenant.json", tenant},
		{"user.json", user},
		{"subscription.json", subAny},
		{"campaigns.json", campaigns},
		{"sequences.json", sequences},
		{"campaign_leads.json", campaignLeads},
		{"email_events.json", emailEvents},
		{"tenant_leads.json", tenantLeads},
		{"gmail_accounts.json", redactedGmail},
		{"unsubscribes.json", unsubs},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := zw.Create(f.name)
		if err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "zip: " + err.Error()})
			return
		}
		if err := json.NewEncoder(fw).Encode(f.data); err != nil {
			apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "encode " + f.name + ": " + err.Error()})
			return
		}
	}
	if err := zw.Close(); err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "export_failed", Message: "zip close: " + err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="magiklead-export-%s.zip"`, tenantID.String()))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("account export: write body for tenant %s: %v", tenantID, err)
	}
}

// Delete handles DELETE /api/v1/account. Order is Clerk first, then
// local cascade — that way a Clerk failure leaves the local data
// intact for a retry, and a local failure after a successful Clerk
// delete is the (loud, logged) divergence we accept as the lesser
// evil. Idempotent: a missing local user returns 204 immediately
// without calling Clerk.
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	if tenantID == uuid.Nil {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	clerkID := middleware.GetUserClerkID(r.Context())
	if clerkID == "" {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	ctx := r.Context()

	user, err := h.queries.GetUserByClerkID(ctx, clerkID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Already gone locally — treat as success so a client
			// retrying after a partial earlier delete (or a sign-in
			// from a stale session) doesn't 500.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "delete_failed", Message: "user lookup: " + err.Error()})
		return
	}

	if err := h.clerk.DeleteUser(ctx, clerkID); err != nil && !errors.Is(err, clerk.ErrNotFound) {
		log.Printf("account delete: clerk failed for tenant %s clerk_id %s: %v", tenantID, clerkID, err)
		apierr.WriteError(w, apierr.APIError{
			Status:  http.StatusBadGateway,
			Code:    "clerk_delete_failed",
			Message: "Clerk user delete failed; no local data was changed. Retry.",
		})
		return
	}

	pgTenant := pgUUID(tenantID)
	if err := h.tx(ctx, func(q cascadeDeleter) error {
		// Order matters: walk most-dependent → least-dependent so each
		// DELETE leaves no orphan rows that would break FK constraints
		// for the next step.
		if err := q.DeleteEmailEventsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("email_events: %w", err)
		}
		if err := q.DeleteCampaignLeadsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("campaign_leads: %w", err)
		}
		if err := q.DeleteCampaignsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("campaigns: %w", err)
		}
		if err := q.DeletePlaysByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("plays: %w", err)
		}
		if err := q.DeleteGmailAccountsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("gmail_accounts: %w", err)
		}
		if err := q.DeleteEmailAccountsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("email_accounts: %w", err)
		}
		if err := q.DeleteSubscriptionByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("subscriptions: %w", err)
		}
		if err := q.DeleteUserTenantsByTenant(ctx, pgTenant); err != nil {
			return fmt.Errorf("user_tenants: %w", err)
		}
		if err := q.DeleteTenantByID(ctx, pgTenant); err != nil {
			return fmt.Errorf("tenants: %w", err)
		}
		if err := q.DeleteUserByIDIfOrphan(ctx, user.ID); err != nil {
			return fmt.Errorf("users: %w", err)
		}
		return nil
	}); err != nil {
		// At this point Clerk has already deleted the user, so the
		// local-side error is the divergence-worthy event — log loudly.
		log.Printf("account delete: local cascade failed AFTER clerk delete for tenant %s: %v", tenantID, err)
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "delete_failed", Message: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
