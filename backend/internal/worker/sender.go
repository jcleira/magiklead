package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/smtp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

type SequenceStep struct {
	Step      int    `json:"step"`
	DelayDays int    `json:"delay_days"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// StartSendLoop runs every 60 seconds, finds leads that are due for their
// next email, and sends via SMTP.
func StartSendLoop(ctx context.Context, queries *repository.Queries) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	log.Println("Email send loop started (checking every 60s)")

	// Run immediately on start, then every tick
	processQueue(ctx, queries)

	for {
		select {
		case <-ctx.Done():
			log.Println("Email send loop stopping")
			return
		case <-ticker.C:
			processQueue(ctx, queries)
		}
	}
}

func processQueue(ctx context.Context, queries *repository.Queries) {
	// Get leads that are due (next_send_at <= NOW, status = active)
	dueLeads, err := queries.GetDueLeads(ctx, 10) // Process 10 at a time
	if err != nil {
		log.Printf("Send loop: error getting due leads: %v", err)
		return
	}

	if len(dueLeads) == 0 {
		return
	}

	log.Printf("Send loop: %d leads due for sending", len(dueLeads))

	for _, lead := range dueLeads {
		if !lead.Email.Valid || lead.Email.String == "" {
			// Skip leads without email — mark exhausted
			queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
				ID:          lead.ID,
				CurrentStep: lead.CurrentStep,
				Status:      pgtype.Text{String: "exhausted", Valid: true},
			})
			continue
		}

		// Get the campaign to read the sequence
		campaign, err := getCampaignForLead(ctx, queries, lead.CampaignID)
		if err != nil {
			log.Printf("Send loop: can't get campaign for lead %s: %v", fmtID(lead.ID), err)
			continue
		}

		// Parse sequence
		var sequence []SequenceStep
		if err := json.Unmarshal(campaign.Sequence, &sequence); err != nil {
			log.Printf("Send loop: can't parse sequence: %v", err)
			continue
		}

		currentStep := int(lead.CurrentStep.Int32)
		if currentStep >= len(sequence) {
			// All steps done — mark exhausted
			queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
				ID:          lead.ID,
				CurrentStep: pgtype.Int4{Int32: int32(currentStep), Valid: true},
				Status:      pgtype.Text{String: "exhausted", Valid: true},
			})
			continue
		}

		step := sequence[currentStep]

		// Get email account for this campaign's tenant
		emailAccounts, err := queries.ListEmailAccounts(ctx, campaign.TenantID)
		if err != nil || len(emailAccounts) == 0 {
			log.Printf("Send loop: no email account for tenant %s", fmtID(campaign.TenantID))
			continue
		}
		account := emailAccounts[0]

		// Personalize the email
		subject := personalize(step.Subject, lead)
		body := personalize(step.Body, lead)

		// Add CAN-SPAM footer
		body += "\n\n---\nSent via MagikLead. If you'd like to stop receiving these emails, reply with 'unsubscribe'."

		// Send
		log.Printf("Sending step %d to %s (%s at %s)", currentStep+1, lead.Email.String, lead.FirstName, lead.Company.String)
		err = sendSMTP(account, lead.Email.String, subject, body)
		if err != nil {
			log.Printf("Send FAILED to %s: %v", lead.Email.String, err)
			// Don't advance step — will retry next cycle
			// If it's a permanent failure (bad email), mark bounced
			if isBounce(err) {
				queries.UpdateCampaignLeadStep(ctx, repository.UpdateCampaignLeadStepParams{
					ID:          lead.ID,
					CurrentStep: pgtype.Int4{Int32: int32(currentStep), Valid: true},
					Status:      pgtype.Text{String: "bounced", Valid: true},
				})
				// Log the bounce
				queries.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
					CampaignLeadID: lead.ID,
					EventType:      "bounced",
					Step:            int32(currentStep + 1),
					Metadata:        []byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())),
				})
			}
			continue
		}

		log.Printf("Sent step %d to %s successfully", currentStep+1, lead.Email.String)

		// Log the sent event
		queries.CreateEmailEvent(ctx, repository.CreateEmailEventParams{
			CampaignLeadID: lead.ID,
			EventType:      "sent",
			Step:            int32(currentStep + 1),
		})

		// Advance to next step
		nextStep := currentStep + 1
		var nextSendAt pgtype.Timestamptz
		var nextStatus pgtype.Text

		if nextStep >= len(sequence) {
			// Last step done
			nextStatus = pgtype.Text{String: "exhausted", Valid: true}
		} else {
			// Schedule next step
			delayDays := sequence[nextStep].DelayDays
			if delayDays < 1 {
				delayDays = 1
			}
			// Add some randomness: +/- 2 hours
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

		// Increment email account daily count
		queries.IncrementEmailSent(ctx, account.ID)

		// Random delay between sends (2-5 minutes)
		delay := time.Duration(120+rand.Intn(180)) * time.Second
		log.Printf("Waiting %s before next send...", delay.Round(time.Second))
		time.Sleep(delay)
	}
}

func sendSMTP(account repository.EmailAccount, to, subject, htmlBody string) error {
	host := account.SmtpHost.String
	port := account.SmtpPort.Int32
	username := account.SmtpUsername.String
	password := account.SmtpPassword.String
	from := account.Email
	senderName := account.SenderName.String
	if senderName == "" {
		senderName = from
	}

	if host == "" || password == "" {
		return fmt.Errorf("SMTP not configured for account %s", from)
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	// Build MIME message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", senderName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("\r\n")

	// Convert plain text body to basic HTML
	htmlContent := "<html><body><p>" + strings.ReplaceAll(htmlBody, "\n", "<br>") + "</p></body></html>"
	msg.WriteString(htmlContent)

	auth := smtp.PlainAuth("", username, password, host)
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg.String()))
}

func personalize(text string, lead repository.GetDueLeadsRow) string {
	firstName := lead.FirstName
	lastName := lead.LastName
	company := ""
	title := ""
	if lead.Company.Valid {
		company = lead.Company.String
	}
	if lead.Title.Valid {
		title = lead.Title.String
	}

	text = strings.ReplaceAll(text, "{{first_name}}", firstName)
	text = strings.ReplaceAll(text, "{{last_name}}", lastName)
	text = strings.ReplaceAll(text, "{{company}}", company)
	text = strings.ReplaceAll(text, "{{title}}", title)
	return text
}

func isBounce(err error) bool {
	msg := err.Error()
	// Auth errors are OUR problem, not a bounce
	if strings.Contains(msg, "535") || strings.Contains(msg, "Username and Password not accepted") {
		return false
	}
	// Recipient bounce indicators
	return strings.Contains(msg, "550") ||
		strings.Contains(msg, "551") ||
		strings.Contains(msg, "552") ||
		strings.Contains(msg, "553") ||
		strings.Contains(msg, "554") ||
		strings.Contains(msg, "User unknown") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such user")
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
