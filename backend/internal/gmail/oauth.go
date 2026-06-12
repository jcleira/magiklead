package gmail

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleRevokeEndpoint is Google's OAuth2 token revocation endpoint.
// Production callers pass this through; tests inject an httptest URL.
const GoogleRevokeEndpoint = "https://oauth2.googleapis.com/revoke"

// GoogleUserinfoEndpoint returns the address of the mailbox that
// granted access, gated by the userinfo.email scope below. The
// callback reads it to store the real From address instead of a
// placeholder. Production uses this const; tests inject an httptest URL.
const GoogleUserinfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo"

// NewOAuthConfig creates the Google OAuth2 config for Gmail access.
func NewOAuthConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes: []string{
			"https://www.googleapis.com/auth/gmail.send",
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
}

// RevokeToken calls Google's revoke endpoint to invalidate an OAuth
// token (access or refresh). After a successful revoke, sends from
// that mailbox will fail and the local row can be deleted.
//
// endpoint is the revoke URL — production passes GoogleRevokeEndpoint,
// tests pass an httptest.Server URL.
//
// A 200 means the token was active and is now revoked. A 400 with
// invalid_token means the token was already revoked or expired — the
// caller's goal (token no longer usable) is already met, so this is
// treated as success. Any other status is surfaced as an error so the
// caller doesn't delete a local row while the grant is still live on
// Google's side.
func RevokeToken(ctx context.Context, endpoint, token string) error {
	if token == "" {
		return nil
	}

	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?"+form.Encode(), nil)
	if err != nil {
		return fmt.Errorf("gmail: build revoke request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gmail: revoke request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		// 400 covers Google's "invalid_token" response for already-
		// revoked or expired tokens — the goal is already met.
		return nil
	}
	return fmt.Errorf("gmail: revoke endpoint returned %d", resp.StatusCode)
}
