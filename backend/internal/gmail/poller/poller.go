// Package poller implements per-mailbox Gmail History API polling
// for the reply-detection (issue #4) and bounce-detection (issue #5)
// slices. The deep module's single entry point is Poller.Tick, which
// takes a connected account (with its saved history-id cursor) and
// returns the new cursor plus the classified inbound events the
// worker should act on.
//
// Classification has three internal buckets — reply, bounce, and
// unrelated — but only reply and bounce surface as emitted events;
// unrelated messages are dropped at the source so callers don't have
// to filter. "Reply" requires the inbound's In-Reply-To (or any
// element of References) to match a Gmail message-id the system
// previously recorded as a send, resolved via the SentLookup
// callback injected by the worker.
//
// The transport seam mirrors the Sender's: the oauth2 Endpoint and
// an optional Gmail-API base URL override are both set from
// NewForTest in unit tests so a local httptest.Server can stand in
// for Google.
package poller

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	gmailv1 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Kind labels the disposition of an inbound message. Reply,
// KindBounceHard and KindBounceSoft surface in Tick's returned events;
// unrelated is the silent fourth bucket (dropped at the source).
// Bounces split into hard/soft at the poller — hard (5.x.x DSN)
// halts the lead immediately; soft (4.x.x) only halts after three
// consecutive on the same address (threshold enforced by
// suppression.RecordSoftBounce).
type Kind string

const (
	KindReply       Kind = "reply"
	KindBounceHard  Kind = "bounce-hard"
	KindBounceSoft  Kind = "bounce-soft"
)

// ClassifiedEvent is what Tick emits per actionable inbound. For
// bounces, DSNStatusCode is the RFC 3464 status (e.g. "5.1.1") and
// OriginalRecipient is the address that failed (from
// Final-Recipient or Original-Recipient in the DSN). For replies,
// those two fields stay empty.
type ClassifiedEvent struct {
	Kind              Kind
	InboundMessageID  string
	InReplyTo         string
	CampaignLeadID    uuid.UUID
	DSNStatusCode     string
	OriginalRecipient string
}

// SentLookup is the worker-injected callback that resolves a Gmail
// message-id we previously emitted (the `gmail_message_id` column on
// email_events) back to its originating campaign_lead. ok=false
// means "we never sent that message-id" — i.e. someone replied to a
// thread we weren't on, and the inbound is unrelated to our outreach.
type SentLookup func(ctx context.Context, gmailMessageID string) (campaignLeadID uuid.UUID, ok bool, err error)

// Account carries the per-mailbox state Tick needs: OAuth tokens
// (refreshed transparently) and the saved history-id cursor. Cursor
// empty means "never polled before" — Tick will bootstrap from
// users.getProfile.
type Account struct {
	Email        string
	AccessToken  string
	RefreshToken string
	TokenExpiry  time.Time
	Cursor       string
}

// Classified errors. The worker inspects these with errors.Is so it
// can log-and-retry transients without misclassifying terminal
// failures.
var (
	// ErrTokenExpired is returned when the access token cannot be
	// refreshed (refresh token revoked / rotated). The worker should
	// mark the account as needing reconnect.
	ErrTokenExpired = errors.New("poller: token expired (refresh failed)")

	// ErrRateLimited is returned for 429s and 5xx responses — both
	// signal "retry later," and the worker treats them the same.
	ErrRateLimited = errors.New("poller: rate limited or transient server error")
)

// Poller is the deep module's entry point. Construct one per
// process; Tick is safe for concurrent use across mailboxes.
type Poller struct {
	oauthConfig *oauth2.Config
	endpoint    string
	lookup      SentLookup
}

// New wires a production Poller that talks to the real Gmail API.
func New(cfg *oauth2.Config, lookup SentLookup) *Poller {
	return &Poller{oauthConfig: cfg, lookup: lookup}
}

// NewForTest wires a Poller pointed at a stubbed Gmail endpoint. The
// endpoint must include the trailing slash because the Gmail SDK
// appends `users/{userId}/...` directly to it.
func NewForTest(cfg *oauth2.Config, endpoint string, lookup SentLookup) *Poller {
	return &Poller{oauthConfig: cfg, endpoint: endpoint, lookup: lookup}
}

// Tick polls one mailbox once. Returns:
//   - newCursor: the historyId to persist on the gmail_accounts row.
//     Empty if err != nil; in that case the caller must NOT advance the
//     saved cursor — next tick retries from the same point.
//   - events: actionable inbound messages classified as reply or
//     bounce. Unrelated inbound (no In-Reply-To match, not a DSN) is
//     dropped here so the worker has no per-event filtering to do.
//   - err: ErrTokenExpired / ErrRateLimited (sentinel-matched by the
//     caller), or a generic wrapped error for network / transport
//     failures.
//
// On the first poll (account.Cursor == ""), Tick bootstraps the
// cursor from users.getProfile and returns no events — we
// intentionally do not replay months of inbox history on connect.
// On history-id rotation (Gmail returns 404 because the cursor is
// stale), Tick re-bootstraps the same way: caller saves the fresh
// cursor and the next tick picks up cleanly.
func (p *Poller) Tick(ctx context.Context, account Account) (string, []ClassifiedEvent, error) {
	svc, err := p.newService(ctx, account)
	if err != nil {
		return "", nil, err
	}

	if account.Cursor == "" {
		return p.bootstrap(ctx, svc)
	}

	resp, err := svc.Users.History.List("me").
		StartHistoryId(parseHistoryID(account.Cursor)).
		HistoryTypes("messageAdded").
		LabelId("INBOX").
		Context(ctx).Do()
	if err != nil {
		if isRotated(err) {
			// Cursor too old — re-bootstrap. The gap is lost but the
			// next tick resumes cleanly from the new cursor.
			return p.bootstrap(ctx, svc)
		}
		return "", nil, classifyAPIError(err)
	}

	events, err := p.classifyHistory(ctx, svc, resp)
	if err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("%d", resp.HistoryId), events, nil
}

// newService constructs a Gmail SDK service bound to the account's
// OAuth tokens. The oauth2 layer refreshes expired access tokens
// transparently; if the refresh itself fails (revoked refresh
// token), the first SDK call surfaces an error we classify as
// ErrTokenExpired.
func (p *Poller) newService(ctx context.Context, account Account) (*gmailv1.Service, error) {
	token := &oauth2.Token{
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		TokenType:    "Bearer",
		Expiry:       account.TokenExpiry,
	}
	tokenSource := p.oauthConfig.TokenSource(ctx, token)

	opts := []option.ClientOption{option.WithTokenSource(tokenSource)}
	if p.endpoint != "" {
		opts = append(opts, option.WithEndpoint(p.endpoint))
	}
	svc, err := gmailv1.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("poller: new service: %w", err)
	}
	return svc, nil
}

// bootstrap reads the mailbox's current historyId from
// users.getProfile and returns it as the new cursor with no events.
// Used both on first poll and after a history-id rotation.
func (p *Poller) bootstrap(ctx context.Context, svc *gmailv1.Service) (string, []ClassifiedEvent, error) {
	prof, err := svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return "", nil, classifyAPIError(err)
	}
	return fmt.Sprintf("%d", prof.HistoryId), nil, nil
}

// classifyHistory walks every newly-added message in the history
// response, fetches the full message for each, and emits a
// ClassifiedEvent only when the message is actionable (reply with
// confirmed match, or a bounce with parseable DSN status code +
// confirmed original send). The history-list response may include
// several HistoryRecord entries per tick; messagesAdded entries
// within them may duplicate the same message-id (Gmail behaviour) —
// we dedupe per-tick so the worker doesn't double-record a single
// reply or bounce.
//
// Format("full") is used (not metadata) so the bounce branch can
// reach into the message/delivery-status MIME part for the Status:
// line. The reply branch only needs headers, but format=full
// returns those too — one fetch shape covers both paths.
func (p *Poller) classifyHistory(ctx context.Context, svc *gmailv1.Service, resp *gmailv1.ListHistoryResponse) ([]ClassifiedEvent, error) {
	seen := map[string]struct{}{}
	var events []ClassifiedEvent

	for _, h := range resp.History {
		for _, added := range h.MessagesAdded {
			if added.Message == nil || added.Message.Id == "" {
				continue
			}
			if _, dup := seen[added.Message.Id]; dup {
				continue
			}
			seen[added.Message.Id] = struct{}{}

			msg, err := svc.Users.Messages.Get("me", added.Message.Id).
				Format("full").
				Context(ctx).Do()
			if err != nil {
				return nil, classifyAPIError(err)
			}

			ev, ok, err := p.classifyMessage(ctx, msg)
			if err != nil {
				return nil, err
			}
			if ok {
				events = append(events, ev)
			}
		}
	}
	return events, nil
}

// classifyMessage applies the bucket assignment to one inbound. The
// caller treats ok=false as "drop" — unrelated and unmatched-bounce
// messages have no emitted event.
func (p *Poller) classifyMessage(ctx context.Context, msg *gmailv1.Message) (ClassifiedEvent, bool, error) {
	if isBounce(msg.Payload) {
		return p.classifyBounce(ctx, msg)
	}

	// Reply detection: try In-Reply-To first, then each token in
	// References. The first lookup-hit wins (matches the PRD's
	// "first-match wins" rule for multi-recipient threading).
	candidates := replyCandidates(msg.Payload)
	for _, c := range candidates {
		leadID, ok, err := p.lookup(ctx, c)
		if err != nil {
			return ClassifiedEvent{}, false, fmt.Errorf("poller: sent-lookup: %w", err)
		}
		if ok {
			return ClassifiedEvent{
				Kind:             KindReply,
				InboundMessageID: msg.Id,
				InReplyTo:        c,
				CampaignLeadID:   leadID,
			}, true, nil
		}
	}
	return ClassifiedEvent{}, false, nil
}

// classifyBounce parses a DSN inbound. Splits hard (5.x.x) from soft
// (4.x.x) via the RFC 3464 Status: line found in the
// message/delivery-status MIME part. Original-Recipient /
// Final-Recipient in the same part gives us the address that
// bounced. The DSN's outer In-Reply-To / References resolve back to
// the Gmail message-id we previously sent — without that match the
// bounce is dropped as unrelated (someone else's DSN landed in this
// mailbox).
//
// Returns ok=false in three cases, all treated as drops:
//   - No parseable Status: line → logged at warn for operator
//     visibility (some DSN variants are non-standard; we'd rather
//     see them in logs than silently route as hard).
//   - No In-Reply-To/References that SentLookup recognises → the
//     bounce belongs to a thread we weren't on.
//   - No DSN content-type was detected (handled by the caller's
//     isBounce gate; this function only runs when isBounce returned
//     true).
func (p *Poller) classifyBounce(ctx context.Context, msg *gmailv1.Message) (ClassifiedEvent, bool, error) {
	statusCode, recipient, ok := parseDSN(msg.Payload)
	if !ok {
		log.Printf("poller: DSN inbound %s lacks parseable Status: line — dropping (operator-visible)", msg.Id)
		return ClassifiedEvent{}, false, nil
	}

	kind, ok := bounceKindFromStatus(statusCode)
	if !ok {
		log.Printf("poller: DSN inbound %s has unrecognised Status: %q — dropping", msg.Id, statusCode)
		return ClassifiedEvent{}, false, nil
	}

	// The DSN normally carries In-Reply-To pointing at the original
	// send. Reuse the reply-candidates extractor so the resolution
	// path matches the reply branch exactly.
	candidates := replyCandidates(msg.Payload)
	for _, c := range candidates {
		leadID, found, err := p.lookup(ctx, c)
		if err != nil {
			return ClassifiedEvent{}, false, fmt.Errorf("poller: bounce sent-lookup: %w", err)
		}
		if found {
			return ClassifiedEvent{
				Kind:              kind,
				InboundMessageID:  msg.Id,
				InReplyTo:         c,
				CampaignLeadID:    leadID,
				DSNStatusCode:     statusCode,
				OriginalRecipient: recipient,
			}, true, nil
		}
	}
	return ClassifiedEvent{}, false, nil
}

// bounceKindFromStatus maps an RFC 3464 status string ("X.Y.Z") to
// the Kind the worker should act on. 5.x.x = hard (permanent),
// 4.x.x = soft (transient). Other classes are not seen in practice
// from Gmail DSNs; if one ever arrives we drop it rather than guess.
func bounceKindFromStatus(status string) (Kind, bool) {
	if len(status) == 0 {
		return "", false
	}
	switch status[0] {
	case '5':
		return KindBounceHard, true
	case '4':
		return KindBounceSoft, true
	default:
		return "", false
	}
}

// parseDSN walks the multipart/report MIME tree to find the
// message/delivery-status part, then pulls Status: and (Original-
// or Final-)Recipient: out of its plain-text body per RFC 3464.
//
// Returns ok=false when no message/delivery-status part exists, or
// when it exists but has no Status: line we can read. Recipient may
// be empty on success (some DSNs omit it); only the status code is
// required for routing.
func parseDSN(payload *gmailv1.MessagePart) (statusCode, recipient string, ok bool) {
	if payload == nil {
		return "", "", false
	}
	body := findDeliveryStatusBody(payload)
	if body == "" {
		return "", "", false
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(strings.ToLower(line), "status:"):
			statusCode = strings.TrimSpace(line[len("status:"):])
		case strings.HasPrefix(strings.ToLower(line), "original-recipient:"):
			if recipient == "" {
				recipient = extractRecipient(line[len("original-recipient:"):])
			}
		case strings.HasPrefix(strings.ToLower(line), "final-recipient:"):
			if recipient == "" {
				recipient = extractRecipient(line[len("final-recipient:"):])
			}
		}
	}
	if statusCode == "" {
		return "", "", false
	}
	return statusCode, recipient, true
}

// findDeliveryStatusBody walks the part tree and returns the
// decoded body of the first message/delivery-status part. Empty
// string means "not found" (caller drops the bounce).
func findDeliveryStatusBody(part *gmailv1.MessagePart) string {
	if part == nil {
		return ""
	}
	if strings.EqualFold(part.MimeType, "message/delivery-status") && part.Body != nil && part.Body.Data != "" {
		raw, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err != nil {
			// Gmail also emits StdEncoding occasionally; try that
			// before giving up so test fixtures using either form
			// both parse correctly.
			raw, err = base64.StdEncoding.DecodeString(part.Body.Data)
			if err != nil {
				return ""
			}
		}
		return string(raw)
	}
	for _, child := range part.Parts {
		if body := findDeliveryStatusBody(child); body != "" {
			return body
		}
	}
	return ""
}

// extractRecipient pulls the address out of an RFC 3464
// recipient line like "rfc822; user@example.com" (the prefix is
// the address-type tag). Returns the bare email; if no semicolon
// separator is present, returns the trimmed value verbatim.
func extractRecipient(value string) string {
	value = strings.TrimSpace(value)
	if i := strings.Index(value, ";"); i >= 0 {
		value = value[i+1:]
	}
	return strings.TrimSpace(value)
}

// replyCandidates extracts the message-ids worth trying against
// SentLookup: In-Reply-To (a single id), then each whitespace-
// separated token in References (a chain back to the thread root).
// Order matters — In-Reply-To wins ties because it points directly
// at the parent, where References can list distant ancestors.
func replyCandidates(payload *gmailv1.MessagePart) []string {
	if payload == nil {
		return nil
	}
	var out []string
	for _, h := range payload.Headers {
		switch {
		case strings.EqualFold(h.Name, "In-Reply-To"):
			if v := strings.TrimSpace(h.Value); v != "" {
				out = append(out, v)
			}
		case strings.EqualFold(h.Name, "References"):
			for _, tok := range strings.Fields(h.Value) {
				if tok != "" {
					out = append(out, tok)
				}
			}
		}
	}
	return out
}

// isBounce detects DSN format (RFC 3464). Gmail surfaces the parsed
// top-level MIME type on payload.MimeType and the full Content-Type
// header (with parameters like report-type=delivery-status) in
// payload.Headers. We check both so the test stub (which puts the
// full Content-Type value in MimeType) and real Gmail (which splits
// them) both classify correctly.
func isBounce(payload *gmailv1.MessagePart) bool {
	if payload == nil {
		return false
	}
	if isDSNContentType(payload.MimeType) {
		return true
	}
	for _, h := range payload.Headers {
		if strings.EqualFold(h.Name, "Content-Type") && isDSNContentType(h.Value) {
			return true
		}
	}
	return false
}

func isDSNContentType(v string) bool {
	v = strings.ToLower(v)
	return strings.Contains(v, "multipart/report") && strings.Contains(v, "delivery-status")
}

// classifyAPIError maps Gmail SDK transport failures to the public
// Err* sentinels so the worker can pattern-match without parsing
// free-text messages.
func classifyAPIError(err error) error {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		switch {
		case apiErr.Code == 401:
			return fmt.Errorf("%w: %s", ErrTokenExpired, apiErr.Message)
		case apiErr.Code == 429:
			return fmt.Errorf("%w: %s", ErrRateLimited, apiErr.Message)
		case apiErr.Code >= 500:
			return fmt.Errorf("%w: %s", ErrRateLimited, apiErr.Message)
		}
	}
	// oauth2 refresh failures arrive as plain errors mentioning the
	// token endpoint; surface as ErrTokenExpired so the worker can
	// mark the account for reconnect.
	msg := err.Error()
	if strings.Contains(msg, "oauth2:") || strings.Contains(msg, "invalid_grant") {
		return fmt.Errorf("%w: %v", ErrTokenExpired, err)
	}
	return fmt.Errorf("poller: gmail api: %w", err)
}

// isRotated reports whether the SDK error means "your history-id is
// older than Gmail's retention window." That's a 404 on history.list
// — Gmail discards history records after ~7 days.
func isRotated(err error) bool {
	if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
		return apiErr.Code == 404
	}
	return false
}

// parseHistoryID converts the textual cursor we persist into the
// uint64 the Gmail SDK expects. Invalid input falls back to 0 — the
// caller has already short-circuited the empty-cursor case via
// bootstrap, so any unparseable value here is a corruption signal
// and triggering a rotation re-bootstrap on the SDK's 404 is the
// safest recovery.
func parseHistoryID(s string) uint64 {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + uint64(c-'0')
	}
	return n
}
