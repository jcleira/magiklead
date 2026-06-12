package gmail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// Service handles the Gmail OAuth flow (auth-URL generation, code
// exchange, account persistence). Actual outbound message delivery
// lives in Sender.
type Service struct {
	oauthConfig *oauth2.Config
	queries     *repository.Queries
	frontendURL string
	// userinfoEndpoint is the URL FetchEmail GETs with the access
	// token. Production: GoogleUserinfoEndpoint. Tests: httptest URL.
	userinfoEndpoint string
}

func NewService(oauthConfig *oauth2.Config, queries *repository.Queries, frontendURL string) *Service {
	return &Service{
		oauthConfig:      oauthConfig,
		queries:          queries,
		frontendURL:      frontendURL,
		userinfoEndpoint: GoogleUserinfoEndpoint,
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

// FetchEmail reads the address of the mailbox that just granted
// access by calling Google's userinfo endpoint with the access token
// (authorized by the userinfo.email scope). This is the From address
// every campaign sends as, so the callback must store the real value
// rather than a placeholder.
//
// A plain bearer GET (not oauthConfig.Client) keeps this hermetic:
// no implicit token-refresh round-trip to Google, so tests can point
// userinfoEndpoint at an httptest server.
func (s *Service) FetchEmail(ctx context.Context, token *oauth2.Token) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.userinfoEndpoint, nil)
	if err != nil {
		return "", fmt.Errorf("gmail: build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("gmail: userinfo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gmail: userinfo endpoint returned %d", resp.StatusCode)
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("gmail: decode userinfo: %w", err)
	}
	if body.Email == "" {
		return "", fmt.Errorf("gmail: userinfo response carried no email")
	}
	return body.Email, nil
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
