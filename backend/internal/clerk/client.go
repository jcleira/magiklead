// Package clerk wraps the small slice of Clerk's Backend API the
// rest of the app needs. Today that's a single call (delete user)
// invoked from the authenticated account-delete handler (issue #11).
// Webhook ingestion already runs in handler/clerk.go via the svix
// library and stays there — this package is for outbound calls.
package clerk

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultBaseURL is Clerk's public Backend API root. Overridable on
// the Client struct for tests.
const DefaultBaseURL = "https://api.clerk.com/v1"

// ErrNotFound is returned by DeleteUser when Clerk reports the user
// no longer exists. Callers treat this as success (idempotent).
var ErrNotFound = errors.New("clerk: user not found")

// httpDoer matches *http.Client. The interface seam lets tests
// substitute a fake transport without spinning up a real listener.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client is the tiny Clerk Backend API wrapper. Use New for the
// production case (real HTTP, real key); construct directly with a
// custom HTTP and BaseURL in tests.
type Client struct {
	HTTP      httpDoer
	BaseURL   string
	SecretKey string
}

// New builds a Client with a 10s-timeout http.Client. An empty
// secretKey is allowed at construction (so the api process boots in
// dev without Clerk wired) — DeleteUser then fails fast with a clear
// error rather than silently 401-ing against Clerk.
func New(secretKey string) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 10 * time.Second},
		BaseURL:   DefaultBaseURL,
		SecretKey: secretKey,
	}
}

// DeleteUser removes the Clerk user with the given clerkID. Returns
// nil on 200/204, ErrNotFound on 404 (idempotent), or a wrapped error
// for any other status. Callers route ErrNotFound back to the same
// 204 they would have returned on success.
func (c *Client) DeleteUser(ctx context.Context, clerkID string) error {
	if c.SecretKey == "" {
		return errors.New("clerk: secret key not configured")
	}
	if clerkID == "" {
		return errors.New("clerk: empty clerk id")
	}

	url := c.BaseURL + "/users/" + clerkID
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("clerk: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("clerk: do request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return ErrNotFound
	default:
		return fmt.Errorf("clerk: unexpected status %d deleting %s", resp.StatusCode, clerkID)
	}
}
