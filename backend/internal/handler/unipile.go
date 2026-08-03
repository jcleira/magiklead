package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/linkedin/pacer"
	"github.com/jcleira/magiklead/backend/internal/linkedin/unipile"
	"github.com/jcleira/magiklead/backend/internal/middleware"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// unipileStateTTL bounds how long a connect flow may stay in flight
// between AuthURL (mint) and the account.connected webhook (verify). The
// metadata is a signed token, not a server-side session, so this expiry
// is the only thing stopping a captured token from being replayed.
const unipileStateTTL = 15 * time.Minute

// freeLinkedInAccountLimit caps connected LinkedIn accounts per tenant.
// The free tier allows exactly one; multi-account is deferred (PRD Out
// of Scope), so this cap currently applies to every plan in this
// release. The gate lives at AuthURL so a second connect is refused
// before any Unipile call or row write.
const freeLinkedInAccountLimit = 1

// unipileService is the slice of *unipile.Module the handler uses.
// Carrying an interface (not the concrete type) keeps the connect,
// webhook, and disconnect flows testable without real Unipile endpoints.
type unipileService interface {
	HostedAuthLink(ctx context.Context, p unipile.HostedAuthParams) (string, error)
	Disconnect(ctx context.Context, unipileAccountID string) error
	ParseWebhook(body []byte) (unipile.Event, error)
	VerifyAuthToken(token string) bool
	WebhookConfigured() bool
	Configured() bool
}

// unipileQueries is the slice of repository.Queries the LinkedIn handler
// uses. An interface keeps the handlers testable with a fake.
type unipileQueries interface {
	GetUserByClerkID(ctx context.Context, clerkID string) (repository.User, error)
	CountConnectedLinkedInAccounts(ctx context.Context, tenantID pgtype.UUID) (int64, error)
	UpsertLinkedInAccount(ctx context.Context, arg repository.UpsertLinkedInAccountParams) (repository.LinkedinAccount, error)
	SetLinkedInAccountStatusByUnipileID(ctx context.Context, arg repository.SetLinkedInAccountStatusByUnipileIDParams) error
	GetLinkedInAccount(ctx context.Context, arg repository.GetLinkedInAccountParams) (repository.LinkedinAccount, error)
	ListLinkedInAccounts(ctx context.Context, tenantID pgtype.UUID) ([]repository.LinkedinAccount, error)
	DeleteLinkedInAccount(ctx context.Context, arg repository.DeleteLinkedInAccountParams) error
	MarkLinkedInAccepted(ctx context.Context, invitationID string) (pgtype.UUID, error)
	CreateLinkedInEvent(ctx context.Context, arg repository.CreateLinkedInEventParams) (repository.LinkedinEvent, error)
	GetLinkedInLeadByChatID(ctx context.Context, chatID string) (pgtype.UUID, error)
}

// replyRecorder is the slice of the suppression module the inbound-message
// path uses to halt a sequence on reply. An interface keeps the handler
// testable without a DB-backed suppression module in the unit suite;
// *suppression.Module satisfies it in production and the integration suite.
type replyRecorder interface {
	RecordReplyByPerson(ctx context.Context, campaignLeadID uuid.UUID, providerMessageID string) error
}

type UnipileHandler struct {
	svc     unipileService
	queries unipileQueries
	// reply halts a sequence when an inbound-message webhook lands. nil in
	// unit tests that don't exercise the message path; wired to the
	// suppression module in production.
	reply replyRecorder
	// stateSecret signs the metadata token (HS256) embedded in the
	// Unipile hosted-auth link and verified when the account.connected
	// webhook echoes it back. Same value as UNIPILE_WEBHOOK_SECRET — the
	// webhook body HMAC and this metadata token share one secret.
	stateSecret []byte
	// frontendURL is where Unipile redirects the browser after the hosted
	// wizard completes (?linkedin=connected|error).
	frontendURL string
	// apiURL is this backend's own public base URL (APP_URL). AuthURL builds
	// the hosted-auth notify_url from it — apiURL + /api/v1/webhooks/unipile —
	// so Unipile has a channel to POST the account.connected callback that
	// echoes our signed metadata back (issue #3, finding A). Empty in
	// environments with no public URL: the notify_url is simply omitted.
	apiURL string
}

func NewUnipileHandler(svc *unipile.Module, q *repository.Queries, reply *suppression.Module, stateSecret []byte, frontendURL, apiURL string) *UnipileHandler {
	return &UnipileHandler{svc: svc, queries: q, reply: reply, stateSecret: stateSecret, frontendURL: frontendURL, apiURL: apiURL}
}

// AuthURL handles GET /api/v1/linkedin/auth-url. It runs behind
// ClerkAuth, so this is where we capture who is connecting: the tenant
// and the real internal user id are signed into the Unipile account
// metadata. The public webhook — which Unipile reaches with no session —
// reads them back from that signed metadata (see Webhook). Because
// Unipile owns the redirect (unlike the Gmail callback we host), the
// binding rides in metadata rather than self-hosted callback state.
func (h *UnipileHandler) AuthURL(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil || !h.svc.Configured() {
		// UNIPILE_API_KEY absent — degrade like PDL: the feature is simply
		// unavailable, no fatal at startup.
		apierr.WriteError(w, apierr.APIError{Status: http.StatusServiceUnavailable, Code: "linkedin_disabled", Message: "LinkedIn integration is not configured"})
		return
	}
	tenantID := getTenantID(r.Context())

	// Free-tier gate, before any Unipile call or row write.
	count, err := h.queries.CountConnectedLinkedInAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	if count >= freeLinkedInAccountLimit {
		apierr.WriteError(w, apierr.APIError{Status: http.StatusForbidden, Code: "linkedin_account_limit", Message: "Your plan allows one connected LinkedIn account"})
		return
	}

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

	meta, err := encodeUnipileState(tenantID, userID, h.stateSecret)
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	params := unipile.HostedAuthParams{
		Metadata:           meta,
		SuccessRedirectURL: h.frontendURL + "/settings?linkedin=connected",
		FailureRedirectURL: h.frontendURL + "/settings?linkedin=error",
		ExpiresOn:          time.Now().Add(unipileStateTTL),
	}
	// Give Unipile a channel to echo the signed metadata back so the
	// account.connected webhook can bind it (issue #3, finding A). Omitted
	// when no public URL is configured rather than sent as a broken relative.
	if h.apiURL != "" {
		params.NotifyURL = strings.TrimRight(h.apiURL, "/") + "/api/v1/webhooks/unipile"
	}
	url, err := h.svc.HostedAuthLink(r.Context(), params)
	if err != nil {
		log.Printf("unipile: hosted-auth link: %v", err)
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, map[string]string{"url": url})
}

// Webhook handles POST /api/v1/webhooks/unipile. This is a PUBLIC route:
// Unipile reaches it with no Clerk session, so authentication is the
// static `Unipile-Auth` header we configure on the Unipile webhook
// (value == UNIPILE_WEBHOOK_SECRET), compared in constant time. Unipile
// does NOT HMAC-sign payloads (its create-webhook API has no signing
// field, only static headers), so the static header is the only auth.
// Tenant binding rides in the signed metadata of the account.connected
// payload. 503 when unconfigured, 401 on bad auth, idempotent on success.
func (h *UnipileHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if h.svc == nil || !h.svc.WebhookConfigured() {
		log.Print("unipile webhook rejected: auth not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	authed := h.svc.VerifyAuthToken(r.Header.Get("Unipile-Auth"))

	ev, err := h.svc.ParseWebhook(body)
	if err != nil {
		log.Printf("unipile webhook parse: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// The hosted-auth notify_url callback (account.connected) arrives WITHOUT
	// our Unipile-Auth header — it is a per-session callback, not a registered
	// webhook with configured headers — and authenticates instead by the
	// signed metadata it carries, which handleAccountConnected verifies
	// (decodeUnipileState, HS256). Every other event comes from a registered
	// webhook and must present the header.
	if !authed && ev.Type != unipile.EventAccountConnected {
		log.Print("unipile webhook auth failed")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	switch ev.Type {
	case unipile.EventAccountConnected:
		h.handleAccountConnected(r, ev)
	case unipile.EventAccountStatus:
		h.handleAccountStatus(r, ev)
	case unipile.EventInvitationAccepted:
		h.handleInvitationAccepted(r, ev)
	case unipile.EventMessageReceived:
		h.handleMessageReceived(r, ev)
	}
	w.WriteHeader(http.StatusOK)
}

// handleMessageReceived is issue #6's reply transition. An inbound message
// the prospect sent (FromSelf is false) halts the sequence: we match the
// chat back to its campaign_lead by the chat id bound at first-DM time and
// hand off to the suppression module, which flips the lead to 'replied',
// writes the 'replied' event, and suppresses the person — all idempotently,
// so a replay (or an overlap with the reconcile poll) is a safe no-op. Our
// own outbound DM echoed back, and a message for a chat we don't recognise,
// are both no-ops. The webhook still answers 200 so Unipile stops retrying.
func (h *UnipileHandler) handleMessageReceived(r *http.Request, ev unipile.Event) {
	if ev.FromSelf {
		return // our own outbound DM echoed back — not a reply
	}
	if ev.ChatID == "" || h.reply == nil {
		return
	}
	leadID, err := h.queries.GetLinkedInLeadByChatID(r.Context(), ev.ChatID)
	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("unipile webhook: inbound message for unknown chat %s — ignoring", ev.ChatID)
		return
	}
	if err != nil {
		log.Printf("unipile webhook: lookup lead for chat %s: %v", ev.ChatID, err)
		return
	}
	if err := h.reply.RecordReplyByPerson(r.Context(), uuid.UUID(leadID.Bytes), ev.MessageID); err != nil {
		log.Printf("unipile webhook: record reply for chat %s (lead %x): %v", ev.ChatID, leadID.Bytes, err)
	}
}

// handleInvitationAccepted is issue #5's acceptance transition. It matches
// the accepted invitation back to the campaign_lead that sent it (by the
// Unipile invitation id bound at send time) and flips it from
// awaiting_accept to active so the LinkedIn DM tick sends the first
// message on its next pass. MarkLinkedInAccepted gates on the
// awaiting_accept state, so a replayed webhook — or one for an invitation
// we never sent — updates nothing and returns no row; we then skip the
// 'accepted' event so the acceptance-rate signal (issue #7) is never
// double-counted. The webhook still answers 200 either way, so Unipile
// stops retrying.
func (h *UnipileHandler) handleInvitationAccepted(r *http.Request, ev unipile.Event) {
	if ev.InvitationID == "" {
		log.Print("unipile webhook: invitation-accepted with no invitation id — ignoring")
		return
	}
	leadID, err := h.queries.MarkLinkedInAccepted(r.Context(), ev.InvitationID)
	if errors.Is(err, pgx.ErrNoRows) {
		log.Printf("unipile webhook: invitation %s accepted but no awaiting_accept lead matched (unknown or already accepted)", ev.InvitationID)
		return
	}
	if err != nil {
		log.Printf("unipile webhook: mark accepted for invitation %s: %v", ev.InvitationID, err)
		return
	}
	if _, err := h.queries.CreateLinkedInEvent(r.Context(), repository.CreateLinkedInEventParams{
		CampaignLeadID:   leadID,
		EventType:        "accepted",
		Step:             1,
		UnipileMessageID: pgtype.Text{String: ev.InvitationID, Valid: true},
	}); err != nil {
		log.Printf("unipile webhook: write accepted event for lead %x: %v", leadID.Bytes, err)
	}
}

// handleAccountConnected recovers the tenant from the signed metadata
// the hosted-auth link carried and upserts the account as active. The
// upsert is idempotent on unipile_account_id, so a replayed webhook
// lands one row.
func (h *UnipileHandler) handleAccountConnected(r *http.Request, ev unipile.Event) {
	tenantID, _, err := decodeUnipileState(ev.Name, h.stateSecret)
	if err != nil {
		log.Printf("unipile webhook: reject account.connected with bad metadata: %v", err)
		return
	}
	if _, err := h.queries.UpsertLinkedInAccount(r.Context(), repository.UpsertLinkedInAccountParams{
		TenantID:         pgUUID(tenantID),
		UnipileAccountID: ev.AccountID,
		Status:           "active",
	}); err != nil {
		log.Printf("unipile webhook: upsert account %s: %v", ev.AccountID, err)
	}
}

// handleAccountStatus records a status the webhook reports against an
// already-known account — one of the two restriction/disconnect detection
// sources (issue #8; the other is a classified send error in the worker). A
// restricted or disconnected status records the provider's reason in
// last_error so Settings can explain it and prompt a reconnect; a recovered
// (active) status clears last_error. Idempotent: re-delivering the same status
// is the same UPDATE. An unknown provider status maps to "" and is left
// untouched rather than guessed.
func (h *UnipileHandler) handleAccountStatus(r *http.Request, ev unipile.Event) {
	status := mapUnipileStatus(ev.Status)
	if status == "" {
		return // unknown provider status — don't clobber the current value
	}
	lastErr := pgtype.Text{}
	if status != "active" {
		lastErr = pgtype.Text{String: ev.Status, Valid: true}
	}
	if err := h.queries.SetLinkedInAccountStatusByUnipileID(r.Context(), repository.SetLinkedInAccountStatusByUnipileIDParams{
		UnipileAccountID: ev.AccountID,
		Status:           status,
		LastError:        lastErr,
	}); err != nil {
		log.Printf("unipile webhook: set status for %s: %v", ev.AccountID, err)
	}
}

// ListAccounts handles GET /api/v1/linkedin/accounts.
func (h *UnipileHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	accounts, err := h.queries.ListLinkedInAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, accounts)
}

// Capacity handles GET /api/v1/linkedin/capacity — the weekly-invite
// capacity indicator behind the dashboard (issue #9). It reports how
// many connection invites the tenant's connected account may still send
// this week: the effective weekly cap (which the warmup ramp lowers for
// a young account) minus the invites already used in the current weekly
// window. Single-account MVP, so it reads the tenant's first account;
// with none connected it returns connected=false and the UI prompts a
// connect rather than rendering a meaningless bar.
//
// This is the pacer's WeeklyRemaining read — the dashboard's "how much
// weekly budget is left" view — not the worker's per-send Allowance,
// which also folds in the daily sub-cap and the acceptance breaker. The
// account's status is returned alongside so the UI can flag a warming or
// restricted account distinctly from one that is simply out of budget.
func (h *UnipileHandler) Capacity(w http.ResponseWriter, r *http.Request) {
	tenantID := getTenantID(r.Context())
	accounts, err := h.queries.ListLinkedInAccounts(r.Context(), pgUUID(tenantID))
	if err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	if len(accounts) == 0 {
		apierr.WriteJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}

	a := accounts[0]
	now := time.Now()
	limits := pacer.Standard()
	counters := pacer.Counters{
		WeeklyCount:       int(a.WeeklyInviteCount),
		WeeklyWindowStart: tsOrZero(a.WeeklyWindowStartedAt),
		WarmupStartedAt:   tsOrZero(a.WarmupStartedAt),
	}
	weeklyCap := limits.EffectiveWeeklyCap(counters.WarmupStartedAt, now)
	weeklyRemaining := limits.WeeklyRemaining(counters, now)

	apierr.WriteJSON(w, http.StatusOK, map[string]any{
		"connected":        true,
		"status":           a.Status,
		"weekly_cap":       weeklyCap,
		"weekly_used":      weeklyCap - weeklyRemaining,
		"weekly_remaining": weeklyRemaining,
	})
}

// DeleteAccount handles DELETE /api/v1/linkedin/accounts/{id}. It looks
// the account up (tenant-scoped), disconnects it at Unipile so the
// session stops acting on the tenant's behalf, then deletes the local
// row. Disconnect failures are logged but don't block the row delete —
// once the user clicks Disconnect they expect the row gone (mirrors the
// Gmail revoke-then-delete contract).
func (h *UnipileHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		apierr.WriteError(w, apierr.ErrBadRequest)
		return
	}
	tenantID := getTenantID(r.Context())

	account, err := h.queries.GetLinkedInAccount(r.Context(), repository.GetLinkedInAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	})
	if err != nil {
		// Already gone or never owned by this tenant — the caller's goal
		// is met. 204 keeps the idempotent-delete contract.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.svc.Disconnect(r.Context(), account.UnipileAccountID); err != nil {
		log.Printf("unipile: disconnect account %s: %v", id, err)
	}

	if err := h.queries.DeleteLinkedInAccount(r.Context(), repository.DeleteLinkedInAccountParams{
		ID:       pgUUID(id),
		TenantID: pgUUID(tenantID),
	}); err != nil {
		apierr.WriteError(w, apierr.ErrInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mapUnipileStatus folds Unipile's account-status vocabulary into our
// status enum. Unknown values return "" so the caller leaves the
// current status untouched rather than guessing.
func mapUnipileStatus(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "OK", "OK_PENDING", "CONNECTED", "CREATION_SUCCESS", "SYNC_SUCCESS":
		return "active"
	case "ERROR", "CREDENTIALS", "STOPPED", "PERMISSIONS":
		return "restricted"
	case "DELETED", "DISCONNECTED":
		return "disconnected"
	default:
		return ""
	}
}

// encodeUnipileState signs the tenant + user binding into the Unipile
// account metadata. It is the only thing the public webhook can trust,
// so it carries an expiry and is HMAC-signed with stateSecret.
func encodeUnipileState(tenantID, userID uuid.UUID, secret []byte) (string, error) {
	return jwt.Encode(jwt.Claims{
		TenantID: tenantID.String(),
		UserID:   userID.String(),
		Exp:      time.Now().Add(unipileStateTTL).Unix(),
	}, secret)
}

// decodeUnipileState verifies the signed metadata and returns the tenant
// + user it carries. Any failure (missing, tampered, expired, or
// unparseable ids) is an error so the webhook rejects it before writing
// a row.
func decodeUnipileState(raw string, secret []byte) (tenantID, userID uuid.UUID, err error) {
	if raw == "" {
		return uuid.Nil, uuid.Nil, errors.New("unipile: empty metadata")
	}
	claims, err := jwt.Decode(raw, secret)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("unipile: verify metadata: %w", err)
	}
	tenantID, err = uuid.Parse(claims.TenantID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("unipile: bad tenant in metadata: %w", err)
	}
	userID, err = uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("unipile: bad user in metadata: %w", err)
	}
	return tenantID, userID, nil
}
