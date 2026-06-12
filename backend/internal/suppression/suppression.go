// Package suppression is the single source of truth for "do not
// email this person." Every send path gates through IsSuppressed
// before the SDK call; every classification path (reply, hard
// bounce, three-consecutive soft bounce, unsubscribe) writes through
// one of the Record* functions.
//
// The module stores suppression state in two places: the unsubscribes
// table (the authoritative "blocked" list, with per-tenant + global
// rows) and the email_events table (per-campaign_lead audit trail).
// IsSuppressed reads from both so a hard bounce or three-consecutive
// soft bounce blocks the next send even if the cascading
// unsubscribes-row write failed transiently.
package suppression

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// SoftBounceThreshold is the consecutive-soft-bounce count that
// upgrades a transient mailbox failure into a permanent suppression.
// "Consecutive" = with no successful 'sent' event in between.
const SoftBounceThreshold = 3

// Reason values written to unsubscribes.reason. Callers should stick
// to these; the column is free-text only for forward compatibility.
const (
	ReasonManual               = "manual"
	ReasonListUnsubscribe      = "list-unsub"
	ReasonSpamComplaint        = "spam-complaint"
	ReasonReply                = "reply"
	ReasonHardBounce           = "hard-bounce"
	ReasonSoftBounceThreshold  = "soft-bounce-threshold"
)

// Module is the suppression deep module. Construct one per process
// with New(pool); IsSuppressed and the Record* methods are safe for
// concurrent use.
type Module struct {
	q *repository.Queries
}

// New returns a Module backed by the given pgxpool.
func New(pool *pgxpool.Pool) *Module {
	return &Module{q: repository.New(pool)}
}

// IsSuppressed reports whether the (tenantID, email) pair must not be
// emailed. Returns the reason string from unsubscribes.reason when a
// row is present; "hard-bounce" or "soft-bounce-threshold" when the
// signal comes from email_events; empty when not suppressed.
//
// An empty email is treated as not suppressed — callers should never
// hand the module a blank, but if they do we don't want to gate every
// blank-email lead globally.
func (m *Module) IsSuppressed(ctx context.Context, tenantID uuid.UUID, email string) (bool, string, error) {
	if email == "" {
		return false, "", nil
	}

	tenant := pgUUID(tenantID)

	reason, err := m.q.LookupSuppression(ctx, repository.LookupSuppressionParams{
		TenantID: tenant,
		Email:    email,
	})
	if err == nil {
		return true, reason, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, "", fmt.Errorf("suppression: lookup unsubscribes: %w", err)
	}

	bounced, err := m.q.HasHardBounceEvent(ctx, repository.HasHardBounceEventParams{
		TenantID: tenant,
		Email:    email,
	})
	if err != nil {
		return false, "", fmt.Errorf("suppression: lookup hard bounce: %w", err)
	}
	if bounced {
		return true, ReasonHardBounce, nil
	}

	count, err := m.q.CountConsecutiveSoftBounces(ctx, repository.CountConsecutiveSoftBouncesParams{
		TenantID: tenant,
		Email:    email,
	})
	if err != nil {
		return false, "", fmt.Errorf("suppression: count soft bounces: %w", err)
	}
	if count >= SoftBounceThreshold {
		return true, ReasonSoftBounceThreshold, nil
	}

	return false, "", nil
}

// RecordReply marks the campaign_lead as replied, writes a 'replied'
// email_event for the audit trail, and adds a per-tenant suppression
// row so future sequences from the same tenant never re-target the
// address. The Gmail message-id is captured on the event row so the
// poller's downstream classifications can chain back to this point.
func (m *Module) RecordReply(ctx context.Context, campaignLeadID uuid.UUID, gmailMessageID string) error {
	lead := pgUUID(campaignLeadID)

	addr, err := m.q.ResolveCampaignLeadAddress(ctx, lead)
	if err != nil {
		return fmt.Errorf("suppression: resolve lead address: %w", err)
	}

	meta, err := json.Marshal(map[string]string{"gmail_message_id": gmailMessageID})
	if err != nil {
		return fmt.Errorf("suppression: marshal metadata: %w", err)
	}

	if _, err := m.q.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
		CampaignLeadID: lead,
		EventType:      "replied",
		Step:           0,
		Metadata:       meta,
		GmailMessageID: pgText(gmailMessageID),
	}); err != nil {
		return fmt.Errorf("suppression: write replied event: %w", err)
	}

	if err := m.q.MarkCampaignLeadReplied(ctx, lead); err != nil {
		return fmt.Errorf("suppression: mark lead replied: %w", err)
	}

	if addr.Email == "" || !addr.TenantID.Valid {
		// No resolvable address — the audit trail is enough; we can't
		// write an unsubscribes row without an email.
		return nil
	}
	if err := m.q.InsertUnsubscribeTenant(ctx, repository.InsertUnsubscribeTenantParams{
		TenantID: addr.TenantID,
		Email:    addr.Email,
		Reason:   ReasonReply,
	}); err != nil {
		return fmt.Errorf("suppression: write reply unsubscribe: %w", err)
	}
	return nil
}

// ClearReply is the re-engage path: lift the reply suppression on a
// campaign_lead and flip its status back to 'active' so the worker
// picks it up next tick. Only the reply reason is reversible —
// hard-bounce / soft-bounce-threshold / manual / list-unsub /
// spam-complaint stay in place even after a re-engage attempt, since
// those signal "address is bad" or "recipient said no," not "the
// auto-responder fired."
//
// Idempotent: if the unsubscribe row has already been cleared (or
// never existed), the campaign_lead is still reactivated.
func (m *Module) ClearReply(ctx context.Context, campaignLeadID uuid.UUID) error {
	lead := pgUUID(campaignLeadID)

	addr, err := m.q.ResolveCampaignLeadAddress(ctx, lead)
	if err != nil {
		return fmt.Errorf("suppression: resolve lead address: %w", err)
	}
	if addr.Email != "" && addr.TenantID.Valid {
		if err := m.q.ClearReplyUnsubscribe(ctx, repository.ClearReplyUnsubscribeParams{
			TenantID: addr.TenantID,
			Email:    addr.Email,
		}); err != nil {
			return fmt.Errorf("suppression: clear reply unsubscribe: %w", err)
		}
	}
	if err := m.q.ReactivateCampaignLead(ctx, lead); err != nil {
		return fmt.Errorf("suppression: reactivate campaign_lead: %w", err)
	}
	return nil
}

// RecordHardBounce writes a 'bounced' email_event for the matching
// campaign_lead (when one exists) and adds a tenant-scoped
// unsubscribes row so further sends to this address from this tenant
// are blocked. extraMeta is merged into the event JSONB so callers
// (the worker bounce path) can capture the DSN status code and any
// other context alongside the audit row.
func (m *Module) RecordHardBounce(ctx context.Context, leadEmail string, tenantID uuid.UUID, extraMeta map[string]string) error {
	if leadEmail == "" {
		return errors.New("suppression: RecordHardBounce: empty email")
	}
	tenant := pgUUID(tenantID)

	meta := map[string]any{"email": leadEmail}
	for k, v := range extraMeta {
		meta[k] = v
	}
	if err := m.writeBounceEvent(ctx, tenant, leadEmail, "bounced", meta); err != nil {
		return err
	}
	if err := m.q.InsertUnsubscribeTenant(ctx, repository.InsertUnsubscribeTenantParams{
		TenantID: tenant,
		Email:    leadEmail,
		Reason:   ReasonHardBounce,
	}); err != nil {
		return fmt.Errorf("suppression: write hard-bounce unsubscribe: %w", err)
	}
	return nil
}

// RecordSoftBounce writes a 'soft-bounce' email_event. If this is the
// third consecutive soft bounce for the (tenant, email) pair (with no
// successful 'sent' event resetting the streak), it also writes a
// tenant-scoped unsubscribes row with reason 'soft-bounce-threshold'.
// Returns the post-write consecutive-soft-bounce count so the caller
// can log/report progress toward the threshold.
//
// extraMeta is merged into the event JSONB alongside {"email": ...,
// "count": <count>} — callers (the worker bounce path) capture the
// DSN status code there.
func (m *Module) RecordSoftBounce(ctx context.Context, leadEmail string, tenantID uuid.UUID, extraMeta map[string]string) (int64, error) {
	if leadEmail == "" {
		return 0, errors.New("suppression: RecordSoftBounce: empty email")
	}
	tenant := pgUUID(tenantID)

	priorCount, err := m.q.CountConsecutiveSoftBounces(ctx, repository.CountConsecutiveSoftBouncesParams{
		TenantID: tenant,
		Email:    leadEmail,
	})
	if err != nil {
		return 0, fmt.Errorf("suppression: count soft bounces: %w", err)
	}
	newCount := priorCount + 1

	meta := map[string]any{"email": leadEmail, "count": newCount}
	for k, v := range extraMeta {
		meta[k] = v
	}
	if err := m.writeBounceEvent(ctx, tenant, leadEmail, "soft-bounce", meta); err != nil {
		return newCount, err
	}

	if newCount >= SoftBounceThreshold {
		if err := m.q.InsertUnsubscribeTenant(ctx, repository.InsertUnsubscribeTenantParams{
			TenantID: tenant,
			Email:    leadEmail,
			Reason:   ReasonSoftBounceThreshold,
		}); err != nil {
			return newCount, fmt.Errorf("suppression: write soft-bounce-threshold unsubscribe: %w", err)
		}
	}
	return newCount, nil
}

// RecordUnsubscribe writes an unsubscribes row. tenantID == nil
// produces a global suppression (CAN-SPAM: the address is then
// blocked across every tenant on the platform). reason should be one
// of the Reason* constants; the column accepts any string for forward
// compat but writers should stay within the enumerated set.
func (m *Module) RecordUnsubscribe(ctx context.Context, email string, tenantID *uuid.UUID, reason string) error {
	if email == "" {
		return errors.New("suppression: RecordUnsubscribe: empty email")
	}
	if reason == "" {
		return errors.New("suppression: RecordUnsubscribe: empty reason")
	}
	if tenantID == nil {
		return m.q.InsertUnsubscribeGlobal(ctx, repository.InsertUnsubscribeGlobalParams{
			Email:  email,
			Reason: reason,
		})
	}
	return m.q.InsertUnsubscribeTenant(ctx, repository.InsertUnsubscribeTenantParams{
		TenantID: pgUUID(*tenantID),
		Email:    email,
		Reason:   reason,
	})
}

// writeBounceEvent attempts to write a bounce event row against the
// most-recently-created campaign_lead in this tenant that resolves to
// the given email. meta is serialised verbatim as the event JSONB —
// callers always include {"email": email} and may add fields like
// {"dsn_status_code": "...", "count": 3}.
//
// If no campaign_lead exists for this address (e.g. the address
// reached the suppression module via a vector other than an active
// campaign), we skip the audit row but still let the caller proceed
// to its unsubscribes write — the suppression invariant is still
// upheld via the unsubscribes table.
func (m *Module) writeBounceEvent(ctx context.Context, tenant pgtype.UUID, email, eventType string, meta map[string]any) error {
	leadID, err := m.q.FindCampaignLeadForEmail(ctx, repository.FindCampaignLeadForEmailParams{
		TenantID: tenant,
		Email:    email,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("suppression: find campaign_lead for %s: %w", email, err)
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("suppression: marshal event metadata: %w", err)
	}
	if _, err := m.q.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
		CampaignLeadID: leadID,
		EventType:      eventType,
		Step:           0,
		Metadata:       metaBytes,
	}); err != nil {
		return fmt.Errorf("suppression: write %s event: %w", eventType, err)
	}
	return nil
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
