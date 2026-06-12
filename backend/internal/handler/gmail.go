package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// gmailStateTTL bounds how long a connect flow may stay in flight
// between AuthURL (mint) and Callback (verify). The state is a signed
// token, not a server-side session, so this expiry is the only thing
// stopping a captured state from being replayed indefinitely.
const gmailStateTTL = 15 * time.Minute

// gmailService is the slice of *gmail.Service the handler uses.
// Carrying an interface (not the concrete type) keeps the OAuth
// callback path testable without real Google endpoints or a database —
// the gap the SQL-stub verification walks left uncovered.
type gmailService interface {
	GetAuthURL(state string) string
	ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error)
	FetchEmail(ctx context.Context, token *oauth2.Token) (string, error)
	SaveAccount(ctx context.Context, tenantID, userID pgtype.UUID, email string, token *oauth2.Token) (*repository.GmailAccount, error)
}

// gmailHandlerQueries is the slice of repository.Queries the Gmail
// handler uses. Carrying an interface (not the concrete type) keeps
// the connect + DeleteAccount flows testable.
type gmailHandlerQueries interface {
	GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error)
	GetGmailAccount(ctx context.Context, arg repository.GetGmailAccountParams) (repository.GmailAccount, error)
	ListGmailAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.GmailAccount, error)
	DeleteGmailAccount(ctx context.Context, arg repository.DeleteGmailAccountParams) error
}

type GmailHandler struct {
	gmailSvc gmailService
	queries  gmailHandlerQueries
	// stateSecret signs the OAuth `state` param (HS256) so the public
	// callback can trust the tenant + user it carries.
	stateSecret []byte
	// frontendURL is where the callback redirects the browser after a
	// connect completes — the saved account (with its tokens) is never
	// rendered to the browser.
	frontendURL string
	// revokeEndpoint is the URL POSTed to during DeleteAccount.
	// Production: gmail.GoogleRevokeEndpoint. Tests: httptest.Server URL.
	revokeEndpoint string
}

func NewGmailHandler(gmailSvc *gmail.Service, q *repository.Queries, stateSecret []byte, frontendURL string) *GmailHandler {
	return &GmailHandler{
		gmailSvc:       gmailSvc,
		queries:        q,
		stateSecret:    stateSecret,
		frontendURL:    frontendURL,
		revokeEndpoint: gmail.GoogleRevokeEndpoint,
	}
}

// AuthURL handles GET /api/v1/gmail/auth-url. It runs behind ClerkAuth,
// so this is where we capture *who* is connecting: the tenant and the
// real internal user id are signed into the OAuth `state`. The public
// callback — which Google reaches with no session — reads them back
// from that signed state (see Callback). Resolving the user id here
// (not in the callback) is what fixes the user_id=tenant_id FK bug.
func (h *GmailHandler) AuthURL(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())

	clerkID := middleware.GetUserClerkID(r.Context())
	if clerkID == "" {
		apierr.WriteError(w, apierr.ErrUnauthorized)
		return
	}
	user, err := h.queries.GetUserByClerkID(r.Context(), clerkID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{Status: 500, Code: "user_not_found", Message: "user lookup failed"})
		return
	}
	userID := uuid.UUID(user.ID.Bytes)

	state, err := encodeGmailState(tenantID, userID, h.stateSecret)
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}

	apierr.WriteJSON(w, http.StatusOK, map[string]string{
		"url":   h.gmailSvc.GetAuthURL(state),
		"state": state,
	})
}

// Callback handles GET /api/v1/gmail/callback. This is a PUBLIC route:
// Google redirects the user's browser here with ?code&state and no
// Authorization header, so it cannot sit behind ClerkAuth. The tenant +
// user binding instead comes from the signed `state` minted in AuthURL,
// and it is validated *before* the code is exchanged so a forged or
// expired state can never trigger a token exchange or persist a row.
//
// Every outcome redirects the browser back to the frontend settings
// page; the callback never renders the saved account as JSON because it
// carries the OAuth access + refresh tokens, which must not land in the
// browser (history, extensions, shoulder-surfing).
func (h *GmailHandler) Callback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")

	tenantID, userID, err := decodeGmailState(r.URL.Query().Get("state"), h.stateSecret)
	if err != nil || code == "" {
		// A bad/expired state, or a consent-denied return (Google sends
		// the state back with ?error and no code). Either way, hand the
		// user back to settings rather than dumping an error body.
		if err != nil {
			log.Printf("gmail: reject callback: %v", err)
		}
		h.redirectSettings(w, r, "error")
		return
	}

	token, err := h.gmailSvc.ExchangeCode(r.Context(), code)
	if err != nil {
		log.Printf("gmail: exchange code: %v", err)
		h.redirectSettings(w, r, "error")
		return
	}

	// Fetch the real mailbox address from Google userinfo (granted via
	// the userinfo.email scope). Without this every campaign would send
	// From: a hard-coded placeholder.
	email, err := h.gmailSvc.FetchEmail(r.Context(), token)
	if err != nil {
		log.Printf("gmail: fetch userinfo email: %v", err)
		h.redirectSettings(w, r, "error")
		return
	}

	if _, err := h.gmailSvc.SaveAccount(r.Context(), pgUUID(tenantID), pgUUID(userID), email, token); err != nil {
		log.Printf("gmail: save account: %v", err)
		h.redirectSettings(w, r, "error")
		return
	}

	h.redirectSettings(w, r, "connected")
}

// redirectSettings sends the browser back to the frontend settings page
// with a status flag the UI can surface (?gmail=connected|error).
func (h *GmailHandler) redirectSettings(w http.ResponseWriter, r *http.Request, status string) {
	http.Redirect(w, r, h.frontendURL+"/settings?gmail="+status, http.StatusFound)
}

// ListAccounts handles GET /api/v1/gmail/accounts
func (h *GmailHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	accounts, err := h.queries.ListGmailAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, accounts)
}

// DeleteAccount handles DELETE /api/v1/gmail/accounts/:id. The flow:
// look the account up (tenant-scoped), revoke its OAuth token at
// Google's revoke endpoint so future sends from that mailbox fail
// fast, then delete the local row so the poller and worker skip it.
//
// Revoke failures are logged but don't block the row delete — once
// the user clicks Disconnect they expect the row to disappear. A
// revoke that fails because Google is briefly unreachable is
// recoverable; a row that won't delete because Google is unreachable
// is a UX dead-end.
func (h *GmailHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())

	account, err := h.queries.GetGmailAccount(r.Context(), repository.GetGmailAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	})
	if err != nil {
		// Already gone or never owned by this tenant — either way the
		// caller's goal is met. 204 keeps the UI happy.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := gmail.RevokeToken(r.Context(), h.revokeEndpoint, account.AccessToken); err != nil {
		log.Printf("gmail: revoke token for account %s: %v", id, err)
	}

	if err := h.queries.DeleteGmailAccount(r.Context(), repository.DeleteGmailAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	}); err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// encodeGmailState signs the tenant + user binding into the OAuth
// state param. It is the only thing the public callback can trust, so
// it carries an expiry and is HMAC-signed with stateSecret.
func encodeGmailState(tenantID, userID uuid.UUID, secret []byte) (string, error) {
	return jwt.Encode(jwt.Claims{
		TenantID: tenantID.String(),
		UserID:   userID.String(),
		Exp:      time.Now().Add(gmailStateTTL).Unix(),
	}, secret)
}

// decodeGmailState verifies the signed state and returns the tenant +
// user it carries. Any failure (missing, tampered, expired, or
// unparseable ids) is an error so the callback rejects it before
// touching the OAuth code.
func decodeGmailState(raw string, secret []byte) (tenantID, userID uuid.UUID, err error) {
	if raw == "" {
		return uuid.Nil, uuid.Nil, errors.New("gmail: empty state")
	}
	claims, err := jwt.Decode(raw, secret)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("gmail: verify state: %w", err)
	}
	tenantID, err = uuid.Parse(claims.TenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("gmail: bad tenant in state: %w", err)
	}
	userID, err = uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("gmail: bad user in state: %w", err)
	}
	return tenantID, userID, nil
}
