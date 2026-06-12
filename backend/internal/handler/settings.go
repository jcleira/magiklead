package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
)

// settingsQueries is the slice of repository.Queries the settings
// handler actually uses. The interface seam keeps the handler
// testable without a DB — production wires the real *repository.Queries.
type settingsQueries interface {
	GetTenant(ctx context.Context, id pgtype.UUID) (repository.Tenant, error)
	GetSubscription(ctx context.Context, tenantID pgtype.UUID) (repository.Subscription, error)
	ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error)
}

// SettingsHandler serves GET /api/v1/settings — the single payload the
// frontend settings page consumes (workspace, plan, usage, email
// accounts). No state of its own; the handler is just a JSON shaper
// over three queries.
type SettingsHandler struct {
	queries settingsQueries
}

// NewSettingsHandler wires the settings handler against the live DB.
func NewSettingsHandler(q *repository.Queries) *SettingsHandler {
	return &SettingsHandler{queries: q}
}

type settingsWorkspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type settingsPlan struct {
	Name               string `json:"name"`
	QuotaLeadsPerMonth int32  `json:"quota_leads_per_month"`
}

type settingsUsage struct {
	LeadsSavedThisPeriod int32 `json:"leads_saved_this_period"`
}

// settingsEmailAccount is the per-mailbox payload the UI renders into a
// row with a status indicator. Provider is hard-coded "gmail" for now
// — Outlook/SMTP are explicitly out of scope per PRD.
type settingsEmailAccount struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Provider     string  `json:"provider"`
	Status       string  `json:"status"`
	LastPolledAt *string `json:"last_polled_at"`
}

type settingsResponse struct {
	Workspace     settingsWorkspace      `json:"workspace"`
	Plan          settingsPlan           `json:"plan"`
	Usage         settingsUsage          `json:"usage"`
	EmailAccounts []settingsEmailAccount `json:"email_accounts"`
}

// Status constants surfaced on each connected mailbox. `scope_missing`
// would surface if the granted scope set lacked gmail.send — today the
// schema doesn't store granted scopes, so the handler returns only
// `connected` and `token_expired`. Adding scope tracking is a follow-up.
const (
	gmailStatusConnected    = "connected"
	gmailStatusTokenExpired = "token_expired"
)

// Get serves the settings payload for the current tenant.
//
//	GET /api/v1/settings
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	if tenantID == uuid.Nil {
		// EnsureTenant middleware should have populated this — reaching
		// here means a routing bug.
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	pgTenant := pgUUID(tenantID)

	tenant, err := h.queries.GetTenant(r.Context(), pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "tenant_lookup_failed", Message: err.Error()})
		return
	}

	plan, usage := h.fetchPlanAndUsage(r.Context(), pgTenant)

	accounts, err := h.queries.ListGmailAccounts(r.Context(), pgTenant)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "gmail_lookup_failed", Message: err.Error()})
		return
	}

	resp := settingsResponse{
		Workspace: settingsWorkspace{
			ID:   uuid.UUID(tenant.ID.Bytes).String(),
			Name: tenant.Name,
		},
		Plan:          plan,
		Usage:         usage,
		EmailAccounts: emailAccountsFromGmail(accounts),
	}

	apierr.WriteJSON(w, http.StatusOK, resp)
}

// fetchPlanAndUsage reads the tenant's subscription row. A missing row
// (pgx.ErrNoRows) degrades to the free-tier defaults — usage zero,
// plan "free", quota 100 — so a brief window between tenant create
// and subscription create doesn't 500 the settings page.
func (h *SettingsHandler) fetchPlanAndUsage(ctx context.Context, tenantID pgtype.UUID) (settingsPlan, settingsUsage) {
	sub, err := h.queries.GetSubscription(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return settingsPlan{Name: "free", QuotaLeadsPerMonth: 100}, settingsUsage{}
		}
		// Any other DB error — log via the JSON error contract but
		// still surface usable defaults so the page renders.
		return settingsPlan{Name: "free", QuotaLeadsPerMonth: 100}, settingsUsage{}
	}

	return settingsPlan{
			Name:               sub.Plan,
			QuotaLeadsPerMonth: sub.LeadsLimit,
		}, settingsUsage{
			LeadsSavedThisPeriod: sub.LeadsUsed.Int32,
		}
}

// emailAccountsFromGmail projects gmail_accounts rows into the UI's
// per-mailbox shape. Provider is hard-coded "gmail" until Outlook or
// SMTP variants come online.
func emailAccountsFromGmail(rows []repository.GmailAccount) []settingsEmailAccount {
	out := make([]settingsEmailAccount, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		acc := settingsEmailAccount{
			ID:       uuid.UUID(row.ID.Bytes).String(),
			Email:    row.Email,
			Provider: "gmail",
			Status:   gmailStatusConnected,
		}
		if row.TokenExpiry.Valid && row.TokenExpiry.Time.Before(now) {
			acc.Status = gmailStatusTokenExpired
		}
		if row.LastPolledAt.Valid {
			s := row.LastPolledAt.Time.UTC().Format(time.RFC3339)
			acc.LastPolledAt = &s
		}
		out = append(out, acc)
	}
	return out
}
