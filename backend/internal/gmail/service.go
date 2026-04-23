package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"mime"
	"net/textproto"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"
	goauth2 "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Service handles Gmail OAuth and email sending.
type Service struct {
	oauthConfig *oauth2.Config
	queries     *repository.Queries
	frontendURL string
}

func NewService(oauthConfig *oauth2.Config, queries *repository.Queries, frontendURL string) *Service {
	return &Service{
		oauthConfig: oauthConfig,
		queries:     queries,
		frontendURL: frontendURL,
	}
}

// GetAuthURL returns the Google OAuth authorization URL.
func (s *Service) GetAuthURL(state string) string {
	return s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// ExchangeCode exchanges an authorization code for tokens.
func (s *Service) ExchangeCode(ctx context.Context, code string) (*oauth2.Token, error) {
	return s.oauthConfig.Exchange(ctx, code)
}

// SaveAccount stores Gmail OAuth tokens in the database.
func (s *Service) SaveAccount(ctx context.Context, tenantID, userID pgtype.UUID, email string, token *oauth2.Token) (*repository.GmailAccount, error) {
	var expiry pgtype.Timestamptz
	if !token.Expiry.IsZero() {
		expiry = pgtype.Timestamptz{Time: token.Expiry, Valid: true}
	}

	account, err := s.queries.CreateGmailAccount(ctx, repository.CreateGmailAccountParams{
		TenantID:     tenantID,
		UserID:       userID,
		Email:        email,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenExpiry:  expiry,
	})
	if err != nil {
		return nil, fmt.Errorf("save gmail account: %w", err)
	}
	return &account, nil
}

// SendEmail sends an email via the Gmail API using stored OAuth tokens.
func (s *Service) SendEmail(ctx context.Context, accountID pgtype.UUID, tenantID pgtype.UUID, to, subject, body, senderName string) error {
	account, err := s.queries.GetGmailAccount(ctx, repository.GetGmailAccountParams{
		ID:       accountID,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("get gmail account: %w", err)
	}

	// Check daily limit
	if account.DailySentCount.Valid && account.DailySentCount.Int32 >= 2000 {
		return fmt.Errorf("daily send limit reached (2000)")
	}

	// Create OAuth2 token
	token := &oauth2.Token{
		AccessToken:  account.AccessToken,
		RefreshToken: account.RefreshToken,
		TokenType:    "Bearer",
	}
	if account.TokenExpiry.Valid {
		token.Expiry = account.TokenExpiry.Time
	}

	// Create token source that auto-refreshes
	tokenSource := s.oauthConfig.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}

	// Update stored tokens if refreshed
	if newToken.AccessToken != token.AccessToken {
		var expiry pgtype.Timestamptz
		if !newToken.Expiry.IsZero() {
			expiry = pgtype.Timestamptz{Time: newToken.Expiry, Valid: true}
		}
		s.queries.UpdateGmailTokens(ctx, repository.UpdateGmailTokensParams{
			ID:           accountID,
			AccessToken:  newToken.AccessToken,
			RefreshToken: newToken.RefreshToken,
			TokenExpiry:  expiry,
		})
	}

	// Create Gmail service
	gmailSvc, err := goauth2.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return fmt.Errorf("create gmail service: %w", err)
	}

	// Build email message
	msg := buildMessage(account.Email, senderName, to, subject, body)

	// Send
	encoded := base64.URLEncoding.EncodeToString(msg)
	_, err = gmailSvc.Users.Messages.Send("me", &goauth2.Message{
		Raw: encoded,
	}).Do()
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	// Increment daily count
	if err := s.queries.IncrementDailySent(ctx, accountID); err != nil {
		log.Printf("failed to increment daily sent count: %v", err)
	}

	return nil
}

// buildMessage creates an RFC 2822 email with CAN-SPAM footer.
func buildMessage(from, fromName, to, subject, htmlBody string) []byte {
	// Add CAN-SPAM footer
	footer := `<div style="margin-top:20px;padding-top:10px;border-top:1px solid #eee;font-size:11px;color:#999;">` +
		`<p>Sent via MagikLead</p></div>`

	fullBody := htmlBody + footer

	header := make(textproto.MIMEHeader)
	header.Set("From", mime.QEncoding.Encode("utf-8", fromName)+" <"+from+">")
	header.Set("To", to)
	header.Set("Subject", mime.QEncoding.Encode("utf-8", subject))
	header.Set("MIME-Version", "1.0")
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Date", time.Now().Format(time.RFC1123Z))

	var msg strings.Builder
	for key, values := range header {
		for _, v := range values {
			msg.WriteString(key)
			msg.WriteString(": ")
			msg.WriteString(v)
			msg.WriteString("\r\n")
		}
	}
	msg.WriteString("\r\n")
	msg.WriteString(fullBody)

	return []byte(msg.String())
}
