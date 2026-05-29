// Sender finishes the TODO that earlier slices left in place: actually
// deliver outbound messages through the user's connected Gmail account
// using the Gmail API, and return the message-id Gmail assigns so that
// the reply / bounce pollers (issues #4, #5) can match incoming
// messages back to the originating campaign_lead.
//
// The Sender is intentionally agnostic about where the OAuth tokens or
// the destination address come from — callers (the worker) hand it a
// ConnectedAccount and a Message; the Sender's only job is to talk to
// the Gmail API correctly and classify whatever comes back. That keeps
// the deep module testable without a DB and without hitting Google.
//
// The transport seam is the Sender's oauth config (whose Endpoint can
// point at an httptest.Server in tests) plus an optional `endpoint`
// override for the Gmail base URL. In production both are real; in
// unit tests both target a local httptest stub.
package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// Classified errors. Callers (the worker) inspect these with errors.Is
// to write the correct event_type / reason on email_events.
var (
	// ErrTokenExpired is returned when the access token cannot be
	// refreshed — the refresh token has been revoked / rotated, and
	// the user must reconnect.
	ErrTokenExpired = errors.New("gmail: token expired (refresh failed)")

	// ErrScopeMissing is returned when Gmail rejects the call because
	// the granted scopes don't cover gmail.send. The UI surfaces a
	// reconnect link in response to this.
	ErrScopeMissing = errors.New("gmail: scope missing — reconnect required")

	// ErrMessageRejected is returned for 4xx responses that aren't
	// scope/auth related — Gmail-side reasons like a malformed
	// address or a too-large payload.
	ErrMessageRejected = errors.New("gmail: message rejected")

	// ErrRateLimited is returned for 429s and 5xx responses — both
	// signal "retry later," and the worker treats them the same way.
	ErrRateLimited = errors.New("gmail: rate limited or transient server error")
)

// ConnectedAccount carries the per-user OAuth state Sender needs to
// authenticate. Constructed by callers from a repository.GmailAccount;
// kept separate from the DB row so Sender can be exercised without a
// DB present.
type ConnectedAccount struct {
	Email        string
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
}

// Message is the outbound payload Sender turns into an RFC 5322
// message. Headers carries any extras the caller needs to set
// (List-Unsubscribe, custom Message-ID prefix, etc); the Sender adds
// From / To / Subject / Date / MIME-Version itself.
type Message struct {
	FromName string
	To       string
	Subject  string
	Body     string // HTML body
	Headers  map[string]string
}

// SendResult is what Send returns on success. MessageID is the value
// that lands on email_events.gmail_message_id for downstream matching.
type SendResult struct {
	MessageID string
	ThreadID  string
}

// Sender is the deep module's entry point. Construct one per process
// with NewSender; Send is safe for concurrent use.
type Sender struct {
	oauthConfig *oauth2.Config
	// endpoint, when non-empty, overrides the Gmail API base URL.
	// Production leaves it empty (uses the real Gmail API); tests
	// point it at an httptest.Server.
	endpoint string
}

// NewSender wires a production Sender that talks to the real Gmail
// API. Tests use NewSenderForTest with a stubbed endpoint.
func NewSender(cfg *oauth2.Config) *Sender {
	return &Sender{oauthConfig: cfg}
}

// NewSenderForTest wires a Sender pointed at a stubbed Gmail endpoint.
// The endpoint must include the trailing slash because the Gmail SDK
// appends `users/{userId}/messages/send` directly to it.
func NewSenderForTest(cfg *oauth2.Config, endpoint string) *Sender {
	return &Sender{oauthConfig: cfg, endpoint: endpoint}
}

// Send delivers msg through Gmail on behalf of account. On success it
// returns the Gmail-assigned messageId and threadId. On failure it
// returns one of the classified Err* sentinels (wrapped with context)
// so callers can branch deterministically.
func (s *Sender) Send(ctx context.Context, account ConnectedAccount, msg Message) (*SendResult, error) {
	if account.AccessToken == "" {
		return nil, fmt.Errorf("gmail: empty access token")
	}
	if msg.To == "" {
		return nil, fmt.Errorf("gmail: empty recipient")
	}

	token := &oauth2.Token{
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       account.TokenExpiry,
	}
	tokenSource := s.oauthConfig.TokenSource(ctx, token)

	opts := []option.ClientOption{option.WithTokenSource(tokenSource)}
	if s.endpoint != "" {
		opts = append(opts, option.WithEndpoint(s.endpoint))
	}

	svc, err := gmailv1.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gmail: new service: %w", err)
	}

	raw := buildRFC5322(account.Email, msg)
	encoded := base64.URLEncoding.EncodeToString(raw)

	resp, err := svc.Users.Messages.Send("me", &gmailv1.Message{Raw: encoded}).Context(ctx).Do()
	if err != nil {
		return nil, classifySendError(err)
	}

	return &SendResult{MessageID: resp.Id, ThreadID: resp.ThreadId}, nil
}

// classifySendError maps Gmail SDK / transport failures to the public
// Err* sentinels. It uses googleapi.Error first (the SDK wraps every
// non-2xx body in one) and falls back to substring matches for
// transport-layer failures the SDK can't classify.
func classifySendError(err error) error {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		switch {
		case apiErr.Code == 401:
			return fmt.Errorf("%w: %s", ErrTokenExpired, apiErr.Message)
		case apiErr.Code == 403 && looksLikeInsufficientScope(apiErr):
			return fmt.Errorf("%w: %s", ErrScopeMissing, apiErr.Message)
		case apiErr.Code == 429:
			return fmt.Errorf("%w: %s", ErrRateLimited, apiErr.Message)
		case apiErr.Code >= 500:
			return fmt.Errorf("%w: %s", ErrRateLimited, apiErr.Message)
		case apiErr.Code >= 400:
			return fmt.Errorf("%w: %s", ErrMessageRejected, apiErr.Message)
		}
	}
	// oauth2 refresh failures arrive as plain errors mentioning the
	// token endpoint; surface them as ErrTokenExpired so the worker
	// can mark the account as needing reconnect.
	if strings.Contains(err.Error(), "oauth2:") || strings.Contains(err.Error(), "token") {
		return fmt.Errorf("%w: %v", ErrTokenExpired, err)
	}
	return fmt.Errorf("gmail: network: %w", err)
}

// looksLikeInsufficientScope inspects a 403 to distinguish a
// scope-missing failure from a generic forbidden. Gmail returns a
// few distinct messages depending on the path; we match the most
// reliable strings and trust the worker's classify-as-rejected
// fallback to handle anything else.
func looksLikeInsufficientScope(apiErr *googleapi.Error) bool {
	m := strings.ToLower(apiErr.Message)
	if strings.Contains(m, "insufficient") && strings.Contains(m, "scope") {
		return true
	}
	if strings.Contains(m, "insufficient authentication scopes") {
		return true
	}
	for _, e := range apiErr.Errors {
		r := strings.ToLower(e.Reason)
		if r == "insufficientpermissions" || r == "forbidden" && strings.Contains(strings.ToLower(e.Message), "scope") {
			return true
		}
	}
	return false
}

// unsubscribeTokenTTL is the lifetime of the signed token embedded in
// the List-Unsubscribe header. RFC 8058 doesn't bound it; 5 years
// matches the issue's "suppression must stay actionable" requirement
// — recipients who unsubscribe today must still be honoured years
// later if the same token surfaces (e.g. an old archived message).
const unsubscribeTokenTTL = 5 * 365 * 24 * time.Hour

// BuildUnsubscribeHeaders returns the RFC 8058 one-click pair the
// worker must attach to every outbound campaign message:
//
//   - List-Unsubscribe: <mailto:unsubscribe+<token>@<mailDomain>>,
//                       <appURL/api/v1/public/unsubscribe?token=<token>>
//   - List-Unsubscribe-Post: List-Unsubscribe=One-Click
//
// The token is the same JWT in both URL slots so a recipient who
// emails the mailto and one who clicks the https link resolve to the
// same suppression record. appURL is the base URL of the api
// (no trailing slash required); mailDomain is the bare hostname used
// for the mailto local-part suffix.
func BuildUnsubscribeHeaders(email string, tenantID uuid.UUID, secret []byte, appURL, mailDomain string) (map[string]string, error) {
	token, err := jwt.Encode(jwt.Claims{
		Email:    email,
		TenantID: tenantID.String(),
		Exp:      time.Now().Add(unsubscribeTokenTTL).Unix(),
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("gmail: encode unsubscribe token: %w", err)
	}

	base := strings.TrimRight(appURL, "/")
	listUnsub := fmt.Sprintf(
		"<mailto:unsubscribe+%s@%s>, <%s/api/v1/public/unsubscribe?token=%s>",
		token, mailDomain, base, token,
	)
	return map[string]string{
		"List-Unsubscribe":      listUnsub,
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}, nil
}

// buildRFC5322 assembles the outbound message bytes. From/To/Subject
// are required; any extra headers in msg.Headers (e.g.
// List-Unsubscribe from issue #6) are appended verbatim. The body is
// always HTML — plain-text alternative parts are out of scope for
// this slice.
func buildRFC5322(fromAddr string, msg Message) []byte {
	header := make(textproto.MIMEHeader)
	fromName := msg.FromName
	if fromName == "" {
		fromName = fromAddr
	}
	header.Set("From", mime.QEncoding.Encode("utf-8", fromName)+" <"+fromAddr+">")
	header.Set("To", msg.To)
	header.Set("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Date", time.Now().Format(time.RFC1123Z))
	for k, v := range msg.Headers {
		header.Set(k, v)
	}

	var b strings.Builder
	for key, values := range header {
		for _, v := range values {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\r\n")
	b.WriteString(msg.Body)
	return []byte(b.String())
}
