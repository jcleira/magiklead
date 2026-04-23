package gmail

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// UnifiedSender sends emails via either SMTP or Gmail OAuth.
type UnifiedSender struct {
	queries     *repository.Queries
	oauthConfig interface{} // Gmail OAuth config (optional)
}

func NewUnifiedSender(queries *repository.Queries) *UnifiedSender {
	return &UnifiedSender{queries: queries}
}

// SendEmail sends an email using the specified email account.
func (s *UnifiedSender) SendEmail(ctx context.Context, accountID, tenantID pgtype.UUID, to, subject, htmlBody string) error {
	account, err := s.queries.GetEmailAccount(ctx, repository.GetEmailAccountParams{
		ID:       accountID,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("get email account: %w", err)
	}

	// Check daily limit
	if account.DailySentCount.Valid && account.DailySentCount.Int32 >= 2000 {
		return fmt.Errorf("daily send limit reached (2000)")
	}

	var sendErr error
	switch account.Provider {
	case "smtp":
		sendErr = s.sendSMTP(account, to, subject, htmlBody)
	case "gmail":
		sendErr = s.sendGmail(ctx, account, to, subject, htmlBody)
	default:
		sendErr = fmt.Errorf("unknown provider: %s", account.Provider)
	}

	if sendErr != nil {
		return sendErr
	}

	// Increment daily count
	s.queries.IncrementEmailSent(ctx, accountID)
	return nil
}

// sendSMTP sends via standard SMTP.
func (s *UnifiedSender) sendSMTP(account repository.EmailAccount, to, subject, htmlBody string) error {
	host := account.SmtpHost.String
	port := account.SmtpPort.Int32
	username := account.SmtpUsername.String
	password := account.SmtpPassword.String
	from := account.Email
	senderName := account.SenderName.String
	if senderName == "" {
		senderName = from
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	// Build the email
	footer := `<div style="margin-top:20px;padding-top:10px;border-top:1px solid #eee;font-size:11px;color:#999;"><p>Sent via MagikLead</p></div>`
	fullBody := htmlBody + footer

	msg := buildMIMEMessage(from, senderName, to, subject, fullBody)

	// Auth
	auth := smtp.PlainAuth("", username, password, host)

	err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	log.Printf("Email sent via SMTP to %s from %s", to, from)
	return nil
}

// sendGmail sends via Gmail API with OAuth tokens.
func (s *UnifiedSender) sendGmail(ctx context.Context, account repository.EmailAccount, to, subject, htmlBody string) error {
	// TODO: Implement Gmail OAuth sending (same as the existing gmail/service.go)
	// For now, fall back to SMTP with Gmail's SMTP settings
	if account.SmtpHost.String == "" {
		// Auto-configure Gmail SMTP
		account.SmtpHost = pgtype.Text{String: "smtp.gmail.com", Valid: true}
		account.SmtpPort = pgtype.Int4{Int32: 587, Valid: true}
		account.SmtpUsername = pgtype.Text{String: account.Email, Valid: true}
		// Password should be a Gmail App Password
		if !account.SmtpPassword.Valid || account.SmtpPassword.String == "" {
			return fmt.Errorf("gmail account needs an App Password for SMTP sending — set it in Settings")
		}
	}
	return s.sendSMTP(account, to, subject, htmlBody)
}

func buildMIMEMessage(from, fromName, to, subject, htmlBody string) string {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", fromName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)
	return msg.String()
}
