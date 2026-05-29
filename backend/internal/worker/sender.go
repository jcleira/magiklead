package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/gmail"
	"github.com/jcleira/magiklead/backend/internal/repository"
	"github.com/jcleira/magiklead/backend/internal/suppression"
)

type SequenceStep struct {
	Step      int    `json:"step"`
	DelayDays int    `json:"delay_days"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// SendFunc is the seam between worker and gmail.Sender. Keeping it as
// a function-shaped seam (rather than a full interface) makes the
// test fake one-line: stubSend := func(...) (*gmail.SendResult, error) { ... }.
// Production wires this with gmail.NewSender(cfg).Send.
type SendFunc func(ctx context.Context, account gmail.ConnectedAccount, msg gmail.Message) (*gmail.SendResult, error)

// UnsubscribeConfig carries everything processQueue needs to mint the
// RFC 8058 List-Unsubscribe headers on every outbound message (issue
// #6). The Secret MUST match the api's UNSUBSCRIBE_SIGNING_SECRET so
// tokens minted here verify at /api/v1/public/unsubscribe.
type UnsubscribeConfig struct {
	Secret     []byte
	AppURL     string // base URL of the api, e.g. http://api-mvp.magiklead.localhost
	MailDomain string // bare hostname for the mailto local-part, e.g. mail.magiklead.com
}

// StartSendLoop runs every 60 seconds, finds leads that are due for
// their next email, and sends via the supplied SendFunc. Every
// outbound message carries the RFC 8058 List-Unsubscribe pair built
// from unsubCfg.
func StartSendLoop(ctx context.Context, queries *repository.Queries, supp *suppression.Module, send SendFunc, unsubCfg UnsubscribeConfig) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	log.Println("Email send loop started (checking every 60s)")

	processQueue(ctx, queries, supp, send, unsubCfg)

	for {
		select {
		case <-ctx.Done():
			log.Println("Email send loop stopping")
			return
		case <-ticker.C:
			processQueue(ctx, queries, supp, send, unsubCfg)
		}
	}
}

func processQueue(ctx context.Context, queries *repository.Queries, supp *suppression.Module, send SendFunc, unsubCfg UnsubscribeConfig) {
	dueLeads, err := queries.GetDueLeads(ctx, 10)
	if err != nil {
		log.Printf("Send loop: error getting due leads: %v", err)
		return
	}
	if len(dueLeads) == 0 {
		return
	}

	log.Printf("Send loop: %d leads due for sending", len(dueLeads))

	for _, lead := range dueLeads {
		if lead.Email == "" {
			queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
				ID:          lead.ID,
				CurrentStep: lead.CurrentStep,
				Status:      pgtype.Text{String: "exhausted", Valid: true},
			})
			continue
		}

		campaign, err := getCampaignForLead(ctx, queries, lead.CampaignID)
		if err != nil {
			log.Printf("Send loop: can't get campaign for lead %s: %v", fmtID(lead.ID), err)
			continue
		}

		var sequence []SequenceStep
		if err := json.Unmarshal(campaign.Sequence, &sequence); err != nil {
			log.Printf("Send loop: can't parse sequence: %v", err)
			continue
		}

		currentStep := int(lead.CurrentStep.Int32)
		if currentStep >= len(sequence) {
			queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
				ID:          lead.ID,
				CurrentStep: pgtype.Int4{Int32: int32(currentStep), Valid: true},
				Status:      pgtype.Text{String: "exhausted", Valid: true},
			})
			continue
		}

		step := sequence[currentStep]

		// Gate every send through the suppression module. A non-nil
		// result halts the send, writes a 'skipped' email_event with
		// the reason, and advances the lead to status='suppressed'.
		tenant := uuid.UUID(campaign.TenantID.Bytes)
		suppressed, reason, err := supp.IsSuppressed(ctx, tenant, lead.Email)
		if err != nil {
			log.Printf("Send loop: suppression check failed for %s: %v — skipping this tick", lead.Email, err)
			continue
		}
		if suppressed {
			meta, _ := json.Marshal(map[string]string{"reason": reason})
			queries.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
				CampaignLeadID: lead.ID,
				EventType:      "skipped",
				Step:           int32(currentStep + 1),
				Metadata:       meta,
			})
			queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
				ID:          lead.ID,
				CurrentStep: lead.CurrentStep,
				Status:      pgtype.Text{String: "suppressed", Valid: true},
			})
			log.Printf("Send loop: skipped %s (%s)", lead.Email, reason)
			continue
		}

		// Pick the tenant's first connected Gmail account. Multi-
		// account-per-tenant is out of scope for the MVP; the operator
		// picks one mailbox per workspace.
		gmailAccounts, err := queries.ListGmailAccounts(ctx, campaign.TenantID)
		if err != nil || len(gmailAccounts) == 0 {
			log.Printf("Send loop: no connected Gmail account for tenant %s", fmtID(campaign.TenantID))
			continue
		}
		account := gmailAccounts[0]

		subject := personalize(step.Subject, lead)
		body := personalize(step.Body, lead)
		// "Sent via MagikLead" branding footer. CAN-SPAM compliance
		// itself is satisfied by the List-Unsubscribe headers below.
		body += `<div style="margin-top:20px;padding-top:10px;border-top:1px solid #eee;font-size:11px;color:#999;"><p>Sent via MagikLead</p></div>`

		// Issue #6: every outbound message carries the RFC 8058
		// one-click headers. If the token mint fails we refuse to
		// send rather than emit a message that lacks the deliverability
		// gate — Gmail would mark such mail as spam.
		unsubHeaders, hdrErr := gmail.BuildUnsubscribeHeaders(lead.Email, tenant, unsubCfg.Secret, unsubCfg.AppURL, unsubCfg.MailDomain)
		if hdrErr != nil {
			log.Printf("Send loop: build unsubscribe headers failed for %s: %v — skipping this tick", lead.Email, hdrErr)
			continue
		}

		log.Printf("Sending step %d to %s (%s at %s)", currentStep+1, lead.Email, lead.FirstName, lead.Company)

		result, sendErr := send(ctx, toConnectedAccount(account), gmail.Message{
			FromName: account.Email,
			To:       lead.Email,
			Subject:  subject,
			Body:     body,
			Headers:  unsubHeaders,
		})
		if sendErr != nil {
			reason := classifyWorkerSendError(sendErr)
			log.Printf("Send FAILED to %s: %v (reason=%s)", lead.Email, sendErr, reason)
			meta, _ := json.Marshal(map[string]string{
				"reason": reason,
				"error":  sendErr.Error(),
			})
			queries.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
				CampaignLeadID: lead.ID,
				EventType:      "failed",
				Step:           int32(currentStep + 1),
				Metadata:       meta,
			})
			continue
		}

		log.Printf("Sent step %d to %s successfully (msg=%s)", currentStep+1, lead.Email, result.MessageID)

		queries.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
			CampaignLeadID: lead.ID,
			EventType:      "sent",
			Step:           int32(currentStep + 1),
			GmailMessageID: pgText(result.MessageID),
		})

		nextStep := currentStep + 1
		var nextSendAt pgtype.Timestamptz
		var nextStatus pgtype.Text

		if nextStep >= len(sequence) {
			nextStatus = pgtype.Text{String: "exhausted", Valid: true}
		} else {
			delayDays := sequence[nextStep].DelayDays
			if delayDays < 1 {
				delayDays = 1
			}
			jitter := time.Duration(rand.Intn(4)-2) * time.Hour
			nextTime := time.Now().Add(time.Duration(delayDays)*24*time.Hour + jitter)
			nextSendAt = pgtype.Timestamptz{Time: nextTime, Valid: true}
			nextStatus = pgtype.Text{String: "active", Valid: true}
		}

		queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
			ID:          lead.ID,
			CurrentStep: pgtype.Int4{Int32: int32(nextStep), Valid: true},
			NextSendAt:  nextSendAt,
			Status:      nextStatus,
		})

		queries.IncrementDailySent(ctx, account.ID)
	}
}

// toConnectedAccount projects a repository.GmailAccount onto the
// minimal shape gmail.Sender needs. Keeping the projection here means
// the gmail package never imports repository, which keeps the deep
// module DB-free for testing.
func toConnectedAccount(a repository.GmailAccount) gmail.ConnectedAccount {
	var expiry time.Time
	if a.TokenExpiry.Valid {
		expiry = a.TokenExpiry.Time
	}
	return gmail.ConnectedAccount{
		Email:        a.Email,
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		TokenExpiry:  expiry,
	}
}

// classifyWorkerSendError turns a gmail.Send error into a short
// reason string for the email_events row. Keeps the metadata column
// queryable by reason without parsing free-text error messages.
func classifyWorkerSendError(err error) string {
	switch {
	case errors.Is(err, gmail.ErrTokenExpired):
		return "token_expired"
	case errors.Is(err, gmail.ErrScopeMissing):
		return "scope_missing"
	case errors.Is(err, gmail.ErrMessageRejected):
		return "rejected"
	case errors.Is(err, gmail.ErrRateLimited):
		return "rate_limited"
	default:
		return "network"
	}
}

func personalize(text string, lead repository.GetDueLeadsRow) string {
	title := ""
	if lead.Title.Valid {
		title = lead.Title.String
	}
	text = strings.ReplaceAll(text, "{{first_name}}", lead.FirstName)
	text = strings.ReplaceAll(text, "{{last_name}}", lead.LastName)
	text = strings.ReplaceAll(text, "{{company}}", lead.Company)
	text = strings.ReplaceAll(text, "{{title}}", title)
	return text
}

func getCampaignForLead(ctx context.Context, queries *repository.Queries, campaignID pgtype.UUID) (*repository.Campaign, error) {
	campaign, err := queries.GetCampaignByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	return &campaign, nil
}

func fmtID(u pgtype.UUID) string {
	if !u.Valid {
		return "nil"
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
