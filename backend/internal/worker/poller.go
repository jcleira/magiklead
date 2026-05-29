// Worker poller tick — calls the gmail/poller deep module for every
// connected mailbox on a 2-minute cadence, then funnels each
// classified event through suppression: replies via RecordReply,
// hard bounces via RecordHardBounce (+ campaign_lead status flip to
// 'bounced'), soft bounces via RecordSoftBounce (which only inserts
// the unsubscribes row once the consecutive-soft-bounce threshold
// is reached). The DSN status code travels in the email_events
// metadata JSONB so the audit trail captures *why* a send was
// suppressed.
package worker

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/gmail/poller"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

// PollFunc is the seam between worker and gmail/poller. Production
// wires this with poller.Poller.Tick; tests pass a one-line stub.
// Keeping it function-shaped (not a full interface) mirrors the
// SendFunc pattern in this package.
type PollFunc func(ctx context.Context, account poller.Account) (newCursor string, events []poller.ClassifiedEvent, err error)

// StartPollLoop runs every 2 minutes (per the PRD's "two-minute
// cadence" decision), polls every connected mailbox, and processes
// any classified events. Cancellation of ctx stops the loop cleanly
// without flushing in-flight ticks.
func StartPollLoop(ctx context.Context, queries *repository.Queries, supp *suppression.Module, poll PollFunc) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	log.Println("Gmail poller loop started (checking every 2m)")

	pollOnce(ctx, queries, supp, poll)

	for {
		select {
		case <-ctx.Done():
			log.Println("Gmail poller loop stopping")
			return
		case <-ticker.C:
			pollOnce(ctx, queries, supp, poll)
		}
	}
}

// pollOnce iterates every connected mailbox once. Failures on one
// account never affect the others — each tick is isolated, the
// cursor is only persisted on a successful poll, and a transient
// outage (rate limit / network error) just means the next tick
// retries from the same saved cursor.
func pollOnce(ctx context.Context, queries *repository.Queries, supp *suppression.Module, poll PollFunc) {
	accounts, err := queries.ListGmailAccountsForPolling(ctx)
	if err != nil {
		log.Printf("Poll loop: list accounts: %v", err)
		return
	}
	if len(accounts) == 0 {
		return
	}
	for _, acc := range accounts {
		tickOne(ctx, queries, supp, poll, acc)
	}
}

// tickOne polls one mailbox. On error: log, return, do NOT advance
// the cursor — that's how the loop survives a transient Gmail
// outage. On success: record any replies through the suppression
// module, then persist the new cursor and last_polled_at so the
// next tick resumes from there.
func tickOne(ctx context.Context, queries *repository.Queries, supp *suppression.Module, poll PollFunc, acc repository.GmailAccount) {
	account := toPollerAccount(acc)
	newCursor, events, err := poll(ctx, account)
	if err != nil {
		// ErrTokenExpired is logged distinctively so the operator can
		// surface a reconnect prompt; the worker doesn't act on it
		// beyond skipping the tick — the UI side of "account needs
		// reconnect" lives in #8 (Settings real data).
		if errors.Is(err, poller.ErrTokenExpired) {
			log.Printf("Poll loop: account %s token expired — needs reconnect", acc.Email)
			return
		}
		log.Printf("Poll loop: tick for %s failed: %v (retrying from saved cursor next tick)", acc.Email, err)
		return
	}

	tenantID := uuid.UUID(acc.TenantID.Bytes)
	for _, ev := range events {
		switch ev.Kind {
		case poller.KindReply:
			if err := supp.RecordReply(ctx, ev.CampaignLeadID, ev.InboundMessageID); err != nil {
				log.Printf("Poll loop: record reply for lead %v failed: %v", ev.CampaignLeadID, err)
				continue
			}
			log.Printf("Poll loop: reply detected for %s (lead=%v, in-reply-to=%s)", acc.Email, ev.CampaignLeadID, ev.InReplyTo)
		case poller.KindBounceHard:
			handleHardBounce(ctx, queries, supp, acc, tenantID, ev)
		case poller.KindBounceSoft:
			handleSoftBounce(ctx, supp, acc, tenantID, ev)
		}
	}

	if err := queries.UpdateGmailAccountCursor(ctx, repository.UpdateGmailAccountCursorParams{
		ID:            acc.ID,
		LastHistoryID: pgText(newCursor),
		LastPolledAt:  pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}); err != nil {
		log.Printf("Poll loop: persist cursor for %s: %v", acc.Email, err)
	}
}

// handleHardBounce records the DSN through the suppression module
// (writes 'bounced' email_event with the DSN status code in
// metadata + inserts the tenant-scoped unsubscribes row) and flips
// the campaign_lead status to 'bounced' so the sender's GetDueLeads
// query stops returning it. Lookup-failures inside the suppression
// module are non-fatal at the tick level — we log and proceed with
// the lead-status flip so the sender stops sending even if the
// audit-trail write hiccupped.
func handleHardBounce(ctx context.Context, queries *repository.Queries, supp *suppression.Module, acc repository.GmailAccount, tenantID uuid.UUID, ev poller.ClassifiedEvent) {
	if ev.OriginalRecipient == "" {
		log.Printf("Poll loop: hard bounce on %s (msg=%s) missing recipient — skipping suppression write", acc.Email, ev.InboundMessageID)
	} else if err := supp.RecordHardBounce(ctx, ev.OriginalRecipient, tenantID, map[string]string{"dsn_status_code": ev.DSNStatusCode}); err != nil {
		log.Printf("Poll loop: record hard-bounce for %s failed: %v", ev.OriginalRecipient, err)
	}
	if ev.CampaignLeadID != uuid.Nil {
		if err := queries.MarkCampaignLeadBounced(ctx, pgtype.UUID{Bytes: ev.CampaignLeadID, Valid: true}); err != nil {
			log.Printf("Poll loop: mark campaign_lead %v bounced failed: %v", ev.CampaignLeadID, err)
		}
	}
	log.Printf("Poll loop: hard bounce on %s recipient=%s status=%s", acc.Email, ev.OriginalRecipient, ev.DSNStatusCode)
}

// handleSoftBounce records the DSN through the suppression module.
// The module increments the consecutive-soft-bounce counter via a
// 'soft-bounce' email_event row (with the DSN status code + count
// in metadata) and only inserts the unsubscribes row at the third
// consecutive hit. Lead status stays 'active' until the next
// IsSuppressed gate in sender.go halts the sequence — soft bounces
// are not lead-terminal until threshold.
func handleSoftBounce(ctx context.Context, supp *suppression.Module, acc repository.GmailAccount, tenantID uuid.UUID, ev poller.ClassifiedEvent) {
	if ev.OriginalRecipient == "" {
		log.Printf("Poll loop: soft bounce on %s (msg=%s) missing recipient — skipping suppression write", acc.Email, ev.InboundMessageID)
		return
	}
	count, err := supp.RecordSoftBounce(ctx, ev.OriginalRecipient, tenantID, map[string]string{"dsn_status_code": ev.DSNStatusCode})
	if err != nil {
		log.Printf("Poll loop: record soft-bounce for %s failed: %v", ev.OriginalRecipient, err)
		return
	}
	log.Printf("Poll loop: soft bounce on %s recipient=%s status=%s (consecutive=%d)", acc.Email, ev.OriginalRecipient, ev.DSNStatusCode, count)
}

// toPollerAccount projects a repository.GmailAccount onto the
// minimal shape the poller needs. Same projection idea as
// toConnectedAccount in sender.go — keeps the poller DB-free.
func toPollerAccount(a repository.GmailAccount) poller.Account {
	var expiry time.Time
	if a.TokenExpiry.Valid {
		expiry = a.TokenExpiry.Time
	}
	var cursor string
	if a.LastHistoryID.Valid {
		cursor = a.LastHistoryID.String
	}
	return poller.Account{
		Email:        a.Email,
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		TokenExpiry:  expiry,
		Cursor:       cursor,
	}
}

// NewSentLookup builds the SentLookup callback the production poller
// uses to resolve "did we send this gmail_message_id?" Wraps the
// FindEmailEventByGmailMessageID query so the poller never imports
// repository directly.
func NewSentLookup(queries *repository.Queries) poller.SentLookup {
	return func(ctx context.Context, gmailMessageID string) (uuid.UUID, bool, error) {
		ev, err := queries.FindEmailEventByGmailMessageID(ctx, pgText(gmailMessageID))
		if err != nil {
			// pgx returns ErrNoRows for unknown ids; the poller
			// treats unknown as "we never sent that" — same as the
			// "not us" branch in the happy path.
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, false, nil
			}
			return uuid.Nil, false, err
		}
		if !ev.CampaignLeadID.Valid {
			return uuid.Nil, false, nil
		}
		return uuid.UUID(ev.CampaignLeadID.Bytes), true, nil
	}
}
