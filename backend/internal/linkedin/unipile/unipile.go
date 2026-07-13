// Package unipile is the deep module behind all LinkedIn access. It
// wraps Unipile — an API-first unified-messaging provider — behind a
// Doer HTTP-client seam (mirroring internal/leads/pdl) so the handler
// and worker never speak HTTP to Unipile directly and the whole
// integration is stubbable in tests. The module owns three concerns:
// minting a hosted-auth wizard link, disconnecting an account, and
// turning inbound webhook payloads into typed events. It holds no
// database handle — persistence is the handler's job.
//
// Per docs/2026-06-09-linkedin-only-outreach/issues/01-connect-linkedin-account.md.
package unipile

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Doer is the HTTP-client seam — net/http.Client satisfies it, and
// tests pass a stub.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Common errors. The handler maps each to the right HTTP response.
var (
	ErrNotConfigured = errors.New("unipile: API key not configured")
	ErrUnauthorized  = errors.New("unipile: unauthorized (401)")
	ErrRateLimited   = errors.New("unipile: rate limited (429)")
	ErrUpstream      = errors.New("unipile: upstream error")
	ErrMalformed     = errors.New("unipile: malformed response")
	// ErrAccountRestricted is returned when LinkedIn (via Unipile) has
	// restricted or checkpointed the connected account. The worker writes
	// it as a 'restricted' failed event; issue #8 escalates the account's
	// status on repeated occurrences.
	ErrAccountRestricted = errors.New("unipile: account restricted")
	// ErrNotFound is returned when a profile cannot be resolved to a member
	// id — the prospect's LinkedIn profile is gone, private, or otherwise
	// unresolvable (a 404, or a 2xx body carrying no provider_id). It is the
	// only permanent resolve failure: the member-id resolver fails the lead
	// terminally on it, and treats every other resolve error as transient.
	ErrNotFound = errors.New("unipile: user not found")
)

// IsPermanentResolveError reports whether a resolve error is permanent — the
// profile will never resolve, so the lead should be failed rather than
// retried. Only ErrNotFound qualifies; rate limits, upstream 5xx, auth, and
// account restrictions are transient and leave the lead queued for a later
// tick. Callers pass the error from ResolveMemberID straight through.
func IsPermanentResolveError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// EventType classifies a parsed webhook payload.
type EventType string

const (
	// EventAccountConnected fires when a hosted-auth wizard completes —
	// Unipile POSTs status CREATION_SUCCESS with the account id and the
	// `name` metadata we set when minting the link.
	EventAccountConnected EventType = "account.connected"
	// EventAccountStatus fires on a later account-status change
	// (restricted, reconnect required, …). Detection from send errors is
	// issue #8; this slice only records the status the webhook reports.
	EventAccountStatus EventType = "account.status"
	// EventInvitationAccepted fires when a prospect accepts a connection
	// request we sent. It carries the invitation id we stored on the
	// campaign_lead at send time (issue #4), which the handler matches back
	// to schedule the first DM (issue #5).
	EventInvitationAccepted EventType = "invitation.accepted"
	// EventMessageReceived fires when a message lands in a chat. It carries
	// the chat id (which the handler matches back to the campaign_lead via
	// linkedin_chat_id) and the message id. FromSelf distinguishes our own
	// outbound DM — echoed back by Unipile — from the prospect's reply; only
	// the latter halts the sequence (issue #6).
	EventMessageReceived EventType = "message.received"
	// EventUnhandled is a well-formed webhook we recognise but do not act
	// on — a messaging event other than message_received (a reaction, a
	// read receipt, …). The handler has no case for it, so the webhook
	// still authenticates, parses, and answers 200 (so Unipile stops
	// retrying) without any side effect.
	EventUnhandled EventType = "unhandled"
)

// Event is the typed view of an inbound Unipile webhook.
type Event struct {
	Type EventType
	// AccountID is the Unipile-side account identifier.
	AccountID string
	// Name carries back the signed pkg/jwt metadata we embedded when the
	// hosted-auth link was minted (set only on EventAccountConnected).
	Name string
	// Status is the raw provider status string on EventAccountStatus.
	Status string
	// InvitationID is the Unipile invitation identifier on
	// EventInvitationAccepted — the key the handler matches back to the
	// campaign_lead that sent the request.
	InvitationID string
	// ChatID and MessageID are set on EventMessageReceived. ChatID is the
	// key the handler matches back to the campaign_lead (linkedin_chat_id);
	// MessageID is the inbound message captured on the 'replied' event.
	ChatID    string
	MessageID string
	// FromSelf is true on EventMessageReceived when the connected account
	// is the sender (our own outbound DM echoed back) rather than the
	// prospect — those must not be treated as a reply.
	FromSelf bool
}

// Module is the Unipile deep module. Construct one per process with
// New. All methods are safe for concurrent use.
type Module struct {
	apiKey        string
	webhookSecret []byte
	doer          Doer
	// baseURL is the per-tenant Unipile DSN (e.g.
	// https://api6.unipile.com:13443). Tests point it at httptest.
	baseURL string
}

// New returns a Module. Pass an empty apiKey to construct a module that
// reports Configured()==false — the api wires one unconditionally and
// lets the handler degrade (like PDL) when no key is set in dev. Pass
// nil for doer to use the default *http.Client.
func New(apiKey, dsn string, webhookSecret []byte, doer Doer) *Module {
	if doer == nil {
		doer = &http.Client{Timeout: 30 * time.Second}
	}
	return &Module{
		apiKey:        apiKey,
		webhookSecret: webhookSecret,
		doer:          doer,
		baseURL:       strings.TrimRight(dsn, "/"),
	}
}

// Configured reports whether the module can call out to Unipile. The
// handler uses this to return a clean "feature disabled" response in
// dev runs without UNIPILE_API_KEY rather than crashing.
func (m *Module) Configured() bool { return m != nil && m.apiKey != "" }

// WebhookConfigured reports whether a webhook secret is set. The public
// webhook handler returns 503 when this is false so unverified payloads
// are never trusted.
func (m *Module) WebhookConfigured() bool { return m != nil && len(m.webhookSecret) > 0 }

// SetBaseURL overrides the Unipile base URL. Tests point it at an
// httptest.Server; production callers leave the DSN passed to New.
func (m *Module) SetBaseURL(u string) { m.baseURL = strings.TrimRight(u, "/") }

// HostedAuthParams configures a hosted-auth wizard link.
type HostedAuthParams struct {
	// Metadata is the signed pkg/jwt token (tenant_id + user_id). Unipile
	// stores it as the account `name` and echoes it back on the
	// account.connected webhook — the only thing binding the connected
	// account to our tenant, since Unipile (not us) owns the redirect.
	Metadata string
	// NotifyURL is the webhook endpoint Unipile POSTs the account.connected
	// callback to once hosted auth completes — the channel that carries the
	// echoed `name` (our signed Metadata) back so the webhook can bind the
	// account to its tenant. Empty ⇒ omitted from the request, and no
	// callback is delivered (issue #3, finding A).
	NotifyURL          string
	SuccessRedirectURL string
	FailureRedirectURL string
	ExpiresOn          time.Time
}

type hostedAuthRequest struct {
	Type               string   `json:"type"`
	Providers          []string `json:"providers"`
	APIURL             string   `json:"api_url"`
	Name               string   `json:"name"`
	NotifyURL          string   `json:"notify_url,omitempty"`
	ExpiresOn          string   `json:"expiresOn,omitempty"`
	SuccessRedirectURL string   `json:"success_redirect_url,omitempty"`
	FailureRedirectURL string   `json:"failure_redirect_url,omitempty"`
}

type hostedAuthResponse struct {
	Object string `json:"object"`
	URL    string `json:"url"`
}

// HostedAuthLink mints a Unipile hosted-auth wizard URL the tenant user
// opens to complete LinkedIn login. The signed Metadata rides in `name`.
func (m *Module) HostedAuthLink(ctx context.Context, p HostedAuthParams) (string, error) {
	if !m.Configured() {
		return "", ErrNotConfigured
	}
	reqBody := hostedAuthRequest{
		Type:               "create",
		Providers:          []string{"LINKEDIN"},
		APIURL:             m.baseURL,
		Name:               p.Metadata,
		NotifyURL:          p.NotifyURL,
		SuccessRedirectURL: p.SuccessRedirectURL,
		FailureRedirectURL: p.FailureRedirectURL,
	}
	if !p.ExpiresOn.IsZero() {
		// Unipile's hosted-auth schema requires millisecond precision
		// (YYYY-MM-DDTHH:MM:SS.sssZ); plain RFC3339 without millis is rejected 400.
		reqBody.ExpiresOn = p.ExpiresOn.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("unipile: marshal hosted-auth request: %w", err)
	}

	resp, err := m.do(ctx, http.MethodPost, "/api/v1/hosted/accounts/link", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return "", err
	}
	var har hostedAuthResponse
	if err := json.Unmarshal(raw, &har); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if har.URL == "" {
		return "", fmt.Errorf("%w: hosted-auth response had no url", ErrMalformed)
	}
	return har.URL, nil
}

// Disconnect tears down a connected account at Unipile so it stops
// acting on the tenant's behalf. Called from the settings DELETE flow.
func (m *Module) Disconnect(ctx context.Context, unipileAccountID string) error {
	if !m.Configured() {
		return ErrNotConfigured
	}
	resp, err := m.do(ctx, http.MethodDelete, "/api/v1/accounts/"+unipileAccountID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return mapStatus(resp.StatusCode, raw)
}

// CancelInvitation withdraws a still-pending connection request at Unipile
// (issue #7's stale-invite withdrawal): a DELETE on the sent-invitation
// resource, scoped to the connected account. LinkedIn ages out unanswered
// invitations and a pile of them counts against an account's standing, so
// the worker cancels any that have gone unaccepted past the horizon. Errors
// classify through the same mapStatus as Disconnect; a 4xx/5xx (e.g. the
// invitation already gone) is returned to the caller, which leaves the lead
// for the next sweep rather than closing it on an unconfirmed withdrawal.
func (m *Module) CancelInvitation(ctx context.Context, accountID, invitationID string) error {
	if !m.Configured() {
		return ErrNotConfigured
	}
	resp, err := m.do(ctx, http.MethodDelete, "/api/v1/users/invite/sent/"+invitationID+"?account_id="+accountID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return mapStatus(resp.StatusCode, raw)
}

// InviteParams is the connection-invite request the worker hands to
// SendInvitation. Recipient is the prospect's Unipile member id (the
// provider_id the invite endpoint addresses), resolved from their
// linkedin_url and cached on the person before the send (issues #5/#6);
// Note is the rendered step-0 connection note (already within LinkedIn's
// character cap, enforced at authoring time in issue #3).
type InviteParams struct {
	AccountID string
	Recipient string
	Note      string
}

// InviteResult is what SendInvitation returns on success. InvitationID
// lands on campaign_leads.linkedin_invitation_id and the invite_sent
// event so the acceptance reconciler (issue #5) can match it back. ChatID
// is normally empty here — no chat exists until the invite is accepted.
type InviteResult struct {
	InvitationID string
	ChatID       string
}

type inviteRequest struct {
	AccountID  string `json:"account_id"`
	ProviderID string `json:"provider_id"`
	Message    string `json:"message,omitempty"`
}

type inviteResponse struct {
	Object       string `json:"object"`
	InvitationID string `json:"invitation_id"`
	ChatID       string `json:"chat_id"`
}

// SendInvitation sends a LinkedIn connection request from the connected
// account to Recipient, carrying the step-0 note. On failure it returns
// one of the classified Err* sentinels (ErrUnauthorized, ErrRateLimited,
// ErrAccountRestricted, ErrUpstream) so the worker can write the right
// failed-event reason — the LinkedIn analogue of gmail.Sender's classified
// returns.
func (m *Module) SendInvitation(ctx context.Context, p InviteParams) (*InviteResult, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	body, err := json.Marshal(inviteRequest{
		AccountID:  p.AccountID,
		ProviderID: p.Recipient,
		Message:    p.Note,
	})
	if err != nil {
		return nil, fmt.Errorf("unipile: marshal invite request: %w", err)
	}

	resp, err := m.do(ctx, http.MethodPost, "/api/v1/users/invite", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := classifySendStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	var ir inviteResponse
	if err := json.Unmarshal(raw, &ir); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return &InviteResult{InvitationID: ir.InvitationID, ChatID: ir.ChatID}, nil
}

// MessageParams is the direct-message request the worker hands to
// SendMessage once an invite has been accepted (issue #5). Recipient is
// the prospect's Unipile member id (the attendee the chat is opened with),
// resolved from their linkedin_url and reused from the person's cache the
// invite populated (issue #6); Text is the rendered DM body. The first DM
// has no chat yet, so SendMessage starts one; later steps thread into the
// returned chat.
type MessageParams struct {
	AccountID string
	Recipient string
	Text      string
}

// MessageResult is what SendMessage returns on success. ChatID is the
// Unipile chat the message landed in — newly created for the first DM —
// recorded on the campaign_lead + dm_sent event so later steps reuse the
// same thread.
type MessageResult struct {
	MessageID string
	ChatID    string
}

type startChatRequest struct {
	AccountID    string   `json:"account_id"`
	AttendeesIDs []string `json:"attendees_ids"`
	Text         string   `json:"text"`
}

type startChatResponse struct {
	Object    string `json:"object"`
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
}

// SendMessage sends the first direct message to a now-connected prospect
// by starting a chat (POST /api/v1/chats) — which creates the conversation
// and posts the message in one call — and returns the new chat id with the
// message id. Errors are classified into the same Err* sentinels as
// SendInvitation so the worker writes the right failed-event reason.
func (m *Module) SendMessage(ctx context.Context, p MessageParams) (*MessageResult, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	body, err := json.Marshal(startChatRequest{
		AccountID:    p.AccountID,
		AttendeesIDs: []string{p.Recipient},
		Text:         p.Text,
	})
	if err != nil {
		return nil, fmt.Errorf("unipile: marshal message request: %w", err)
	}

	resp, err := m.do(ctx, http.MethodPost, "/api/v1/chats", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if err := classifySendStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	var sr startChatResponse
	if err := json.Unmarshal(raw, &sr); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return &MessageResult{MessageID: sr.MessageID, ChatID: sr.ChatID}, nil
}

// AccountActivity is the reconcile backstop's view of one connected
// account (issue #6): the inbound replies Unipile currently shows, so the
// poll can apply any a webhook never delivered. Acceptance is detected
// separately, by AccountConnections (issue #7). Best-effort — the caller
// applies each reply idempotently against the same gate the webhook uses.
type AccountActivity struct {
	Replies []InboundReply
}

// InboundReply is a prospect reply the reconcile poll spotted: the chat it
// landed in (matched to the campaign_lead by linkedin_chat_id) and the
// message id captured on the 'replied' event.
type InboundReply struct {
	ChatID    string
	MessageID string
}

// chatListResponse is the permissive shape of the real GET /api/v1/chats
// envelope (spike §4). The list carries no per-chat direction field and no
// last_message object — direction is per-message only. Its one inbound signal
// is unread_count: a chat accrues unread count only from the other party's
// messages, never our own sends, so unread_count>0 is the same "not from us"
// signal the webhook derives from the member-id cross-check — an inbound reply
// the webhook may have missed.
type chatListResponse struct {
	Items []struct {
		ID          string `json:"id"`
		UnreadCount int    `json:"unread_count"`
	} `json:"items"`
}

// AccountActivity lists, for one connected account, the inbound replies
// Unipile currently shows — the reconcile backstop for reply webhooks that
// never arrived. The chats shape follows Unipile's messaging API and is
// parsed permissively, so an added field never breaks a tick; the reconcile
// caller applies every reply idempotently. A listing failure returns the
// error, and the tick logs and moves on.
func (m *Module) AccountActivity(ctx context.Context, unipileAccountID string) (AccountActivity, error) {
	var out AccountActivity
	if !m.Configured() {
		return out, ErrNotConfigured
	}

	if err := m.list(ctx, "/api/v1/chats?account_id="+unipileAccountID, func(raw []byte) error {
		var resp chatListResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return fmt.Errorf("%w: %v", ErrMalformed, err)
		}
		for _, it := range resp.Items {
			// unread_count>0 ⇒ the chat holds an inbound message (our own sends
			// never accrue unread count) — the list-level equivalent of the
			// webhook's "sender != our own member id" direction check. The list
			// exposes no message id, so the caller records the reply without one;
			// the idempotency gate keys on lead status, not the message id.
			if it.UnreadCount <= 0 {
				continue
			}
			out.Replies = append(out.Replies, InboundReply{ChatID: it.ID})
		}
		return nil
	}); err != nil {
		return out, err
	}
	return out, nil
}

// maxConnectionPages bounds the relations walk so a pathological or broken
// upstream cursor can never loop forever. At trial volume an account's
// connections fit in one or two pages; the cap only guards the walk.
const maxConnectionPages = 100

// userRelationsList is the permissive shape of GET /users/relations: the
// connection member ids plus the pagination cursor (empty or null on the last
// page). member_id is the accept-matcher key (issue #7).
type userRelationsList struct {
	Items []struct {
		MemberID string `json:"member_id"`
	} `json:"items"`
	Cursor string `json:"cursor"`
}

// AccountConnections lists the Unipile member ids currently connected to one
// account (issue #7) — the accept check's input: the reconcile intersects this
// set with the account's awaiting_accept leads to detect acceptances, the sole
// acceptance path (the invitation.accepted notification lags ~8h). It walks
// every page of the relations listing via the envelope cursor so a large
// account's connections come back complete; the shape follows Unipile's
// GET /users/relations and is parsed permissively. A page failure returns the
// error, and the reconcile tick logs and moves on.
func (m *Module) AccountConnections(ctx context.Context, unipileAccountID string) ([]string, error) {
	if !m.Configured() {
		return nil, ErrNotConfigured
	}
	var memberIDs []string
	cursor := ""
	for range maxConnectionPages {
		path := "/api/v1/users/relations?account_id=" + url.QueryEscape(unipileAccountID)
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		var next string
		if err := m.list(ctx, path, func(raw []byte) error {
			var rel userRelationsList
			if err := json.Unmarshal(raw, &rel); err != nil {
				return fmt.Errorf("%w: %v", ErrMalformed, err)
			}
			for _, it := range rel.Items {
				if it.MemberID != "" {
					memberIDs = append(memberIDs, it.MemberID)
				}
			}
			next = rel.Cursor
			return nil
		}); err != nil {
			return nil, err
		}
		if next == "" {
			break
		}
		cursor = next
	}
	return memberIDs, nil
}

// list GETs a Unipile collection endpoint and hands the raw body to parse.
func (m *Module) list(ctx context.Context, path string, parse func(raw []byte) error) error {
	resp, err := m.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if err := mapStatus(resp.StatusCode, raw); err != nil {
		return err
	}
	return parse(raw)
}

// classifySendStatus maps a Unipile send response (invite or message) to
// the classified send sentinels. The restriction check runs before the
// bare status mapping so a 4xx carrying a restriction/checkpoint payload
// reads as ErrAccountRestricted rather than ErrUpstream.
func classifySendStatus(code int, raw []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	if isRestriction(raw) {
		return fmt.Errorf("%w: %s", ErrAccountRestricted, raw)
	}
	switch code {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: http %d: %s", ErrUpstream, code, raw)
	}
}

// isRestriction spots an account-restriction / checkpoint payload by the
// tokens Unipile uses for it. Substring match on the lower-cased body —
// deliberately permissive: a missed classification merely downgrades to
// ErrUpstream (a retryable reason) instead of the restricted escalation
// issue #8 wants, which is the safe direction to err.
func isRestriction(raw []byte) bool {
	b := bytes.ToLower(raw)
	return bytes.Contains(b, []byte("restrict")) || bytes.Contains(b, []byte("checkpoint"))
}

// resolveResponse is the slice of the §6 UserProfile shape the resolver
// needs. provider_id is the Unipile member id (the ACoAA… key every send is
// addressed by). member_urn is deliberately absent: in this response it is a
// numeric urn, unlike the ACoAA… member_urn the connections list carries —
// resolving on it would yield the wrong id.
type resolveResponse struct {
	ProviderID string `json:"provider_id"`
}

// ResolveMemberID turns a prospect's LinkedIn profile URL into their Unipile
// member id through the given connected account (spike §6):
//
//	GET /api/v1/users/<public_identifier>?account_id=<acc>
//
// which returns a bare UserProfile whose provider_id is the member id. The
// error is classified for the caller: ErrNotFound is permanent (the profile
// is gone/private, or the URL carries no /in/ segment); every other error
// (rate limit, upstream 5xx, auth, restriction) is transient. Use
// IsPermanentResolveError to branch on it.
func (m *Module) ResolveMemberID(ctx context.Context, profileURL, accountID string) (string, error) {
	if !m.Configured() {
		return "", ErrNotConfigured
	}
	publicID := publicIdentifierFromURL(profileURL)
	if publicID == "" {
		return "", fmt.Errorf("%w: no public identifier in %q", ErrNotFound, profileURL)
	}
	path := "/api/v1/users/" + url.PathEscape(publicID) + "?account_id=" + url.QueryEscape(accountID)
	resp, err := m.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if err := classifyResolveStatus(resp.StatusCode, raw); err != nil {
		return "", err
	}
	var rr resolveResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		return "", fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if rr.ProviderID == "" {
		return "", fmt.Errorf("%w: resolve response carried no provider_id", ErrNotFound)
	}
	return rr.ProviderID, nil
}

// classifyResolveStatus maps a resolve HTTP status to an error. Only 404 is
// permanent (ErrNotFound → the resolver fails the lead). 401/429/5xx and any
// other non-2xx are transient (the lead stays queued): resolving is a read,
// and an upstream blip must not burn a prospect. A restriction/checkpoint
// payload surfaces as ErrAccountRestricted, also transient here.
func classifyResolveStatus(code int, raw []byte) error {
	if code >= 200 && code < 300 {
		return nil
	}
	switch {
	case code == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, raw)
	case code == http.StatusUnauthorized:
		return ErrUnauthorized
	case code == http.StatusTooManyRequests:
		return ErrRateLimited
	case isRestriction(raw):
		return fmt.Errorf("%w: %s", ErrAccountRestricted, raw)
	default:
		return fmt.Errorf("%w: http %d: %s", ErrUpstream, code, raw)
	}
}

// publicIdentifierFromURL pulls the public identifier — the <slug> in
// linkedin.com/in/<slug> — out of a stored linkedin_url, tolerating the
// scheme, an optional www, a trailing slash, deeper path segments, and any
// query string or fragment. It returns "" when the URL carries no /in/
// segment, which the resolver treats as a permanent failure.
func publicIdentifierFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	const marker = "/in/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return ""
	}
	slug := strings.Trim(raw[i+len(marker):], "/")
	slug, _, _ = strings.Cut(slug, "/") // guard against /in/<slug>/detail/…
	return slug
}

// wireEventMessageReceived is the value of the top-level "event" field on
// the real messaging webhook (spike §9). It is the wire marker Unipile
// sends, distinct from the internal EventMessageReceived classification.
const wireEventMessageReceived = "message_received"

// webhookPayload is the wire shape of an inbound Unipile webhook. Unipile
// sends two disjoint families, told apart by their real top-level marker
// (confirmed against the bytes captured in the #1 spike):
//
//   - messaging events carry a top-level "event" string on a FLAT body
//     (e.g. "message_received"); direction is the bool is_sender.
//   - account-status events nest the status under an "AccountStatus" object.
//
// The hosted-auth connect callback carries the signed `name` metadata and,
// per the #1 spike (finding A), no distinguishing marker of its own — it is
// the fallback once the two real markers are absent (the notify_url connect
// payload issue #3 wires lands here).
//
// is_sender is typed bool because that is what the webhook sends (spike §9);
// the REST /chats surface sends it as an int (chatListResponse). The two
// surfaces are typed separately on purpose (finding F) — a single shared
// struct 400'd one side.
type webhookPayload struct {
	Event        string `json:"event"`
	Status       string `json:"status"`
	AccountID    string `json:"account_id"`
	Name         string `json:"name"`
	InvitationID string `json:"invitation_id"`
	ChatID       string `json:"chat_id"`
	MessageID    string `json:"message_id"`
	// IsSender is the webhook's direction flag: true when the connected
	// account sent the message (our own outbound DM echoed back) rather than
	// the prospect. It is only a fallback — the authoritative direction signal
	// is the member-id cross-check below (spike finding D). Absent reads as
	// false → inbound.
	IsSender bool `json:"is_sender"`
	// AccountInfo.UserID is the connected account's own member id; Sender
	// .AttendeeProviderID is the message sender's member id (both spike §9).
	// Equal ⇒ the message is our own outbound DM echoed back — the real
	// direction signal, robust to is_sender disagreeing (issue #8).
	AccountInfo struct {
		UserID string `json:"user_id"`
	} `json:"account_info"`
	Sender struct {
		AttendeeProviderID string `json:"attendee_provider_id"`
	} `json:"sender"`
	AccountStatus *struct {
		AccountID string `json:"account_id"`
		Type      string `json:"account_type"`
		Message   string `json:"message"`
	} `json:"AccountStatus"`
}

// messageFromSelf decides whether a message_received webhook is the connected
// account's own outbound DM (echoed back by Unipile) rather than a prospect
// reply. The authoritative signal is the member-id cross-check (spike finding
// D): the sender's member id vs the account's own member id — equal ⇒ ours.
// It intentionally overrides the presumed is_sender bool, which is only
// consulted when either member id is absent from the payload.
func messageFromSelf(p webhookPayload) bool {
	own := p.AccountInfo.UserID
	sender := p.Sender.AttendeeProviderID
	if own != "" && sender != "" {
		return sender == own
	}
	return p.IsSender
}

// ParseWebhook turns a raw Unipile webhook body into a typed Event, routing
// on Unipile's real top-level marker (confirmed in the #1 spike), not on
// object-presence guesswork:
//
//   - a top-level "event" string ⇒ a messaging webhook; "message_received"
//     is the only one we act on, the rest classify as EventUnhandled so the
//     handler answers 200 without acting.
//   - an "AccountStatus" block ⇒ an account-status change.
//   - an invitation_id ⇒ an acceptance (issue #5's seam).
//   - otherwise ⇒ a hosted-auth connect completion, carrying the signed
//     metadata in `name` (issue #3 binds it).
func (m *Module) ParseWebhook(body []byte) (Event, error) {
	var p webhookPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if p.Event != "" {
		if p.Event == wireEventMessageReceived {
			return Event{
				Type:      EventMessageReceived,
				AccountID: p.AccountID,
				ChatID:    p.ChatID,
				MessageID: p.MessageID,
				FromSelf:  messageFromSelf(p),
			}, nil
		}
		return Event{Type: EventUnhandled, AccountID: p.AccountID}, nil
	}
	if p.AccountStatus != nil {
		return Event{
			Type:      EventAccountStatus,
			AccountID: p.AccountStatus.AccountID,
			Status:    p.AccountStatus.Message,
		}, nil
	}
	if p.InvitationID != "" {
		return Event{
			Type:         EventInvitationAccepted,
			AccountID:    p.AccountID,
			InvitationID: p.InvitationID,
		}, nil
	}
	return Event{
		Type:      EventAccountConnected,
		AccountID: p.AccountID,
		Name:      p.Name,
	}, nil
}

// VerifyAuthToken constant-time compares the static auth token Unipile
// echoes in the configured `Unipile-Auth` header against the webhook
// secret. This is the ONLY webhook auth path: real Unipile webhooks
// authenticate with a static custom header, not a body HMAC (its
// create-webhook API has no signing field).
func (m *Module) VerifyAuthToken(token string) bool {
	if !m.WebhookConfigured() || token == "" {
		return false
	}
	return hmac.Equal([]byte(token), m.webhookSecret)
}

func (m *Module) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, m.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("unipile: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-KEY", m.apiKey)
	resp, err := m.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unipile: do request: %w", err)
	}
	return resp, nil
}

func mapStatus(code int, raw []byte) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusUnauthorized:
		return ErrUnauthorized
	case code == http.StatusTooManyRequests:
		return ErrRateLimited
	default:
		return fmt.Errorf("%w: http %d: %s", ErrUpstream, code, raw)
	}
}
