// PublicUnsubscribeHandler serves the RFC 8058 one-click endpoint that
// Gmail's deliverability checks require behind every outbound campaign
// message. The token is the JWT minted by gmail.BuildUnsubscribeHeaders
// at send time; this handler validates it and routes the recorded
// unsubscribe through the suppression module so the worker's next tick
// skips the lead.
//
// The route is intentionally mounted outside the Clerk auth chain:
// recipients clicking from their inbox have no Clerk session, and
// authentication here is the signed token, not the bearer header.
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/suppression"
	apierr "github.com/jcleira/magiklead/backend/pkg/errors"
	"github.com/jcleira/magiklead/backend/pkg/jwt"
)

// unsubscribeRecorder is the seam between the handler and
// suppression.Module. Keeping it as an interface (rather than a
// concrete *Module) lets the unit tests substitute a fake that
// records calls without spinning up Postgres.
type unsubscribeRecorder interface {
	RecordUnsubscribe(ctx context.Context, email string, tenantID *uuid.UUID, reason string) error
}

// PublicUnsubscribeHandler validates a signed unsubscribe token and
// writes the suppression row. Construct one per process with
// NewPublicUnsubscribeHandler.
type PublicUnsubscribeHandler struct {
	secret   []byte
	recorder unsubscribeRecorder
}

func NewPublicUnsubscribeHandler(secret []byte, recorder unsubscribeRecorder) *PublicUnsubscribeHandler {
	return &PublicUnsubscribeHandler{secret: secret, recorder: recorder}
}

// Handle serves both GET and POST under the same path. POST is the
// RFC 8058 one-click action; GET is the email-client-pre-fetch /
// hand-typed fallback. Both run the same validation and DB write.
func (h *PublicUnsubscribeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		apierr.WriteError(w, apierr.APIError{
			Status: http.StatusUnauthorized, Code: "invalid_token",
			Message: "missing or invalid unsubscribe token",
		})
		return
	}

	claims, err := jwt.Decode(rawToken, h.secret)
	switch {
	case errors.Is(err, jwt.ErrExpired):
		apierr.WriteError(w, apierr.APIError{
			Status: http.StatusGone, Code: "token_expired",
			Message: "unsubscribe link has expired",
		})
		return
	case err != nil:
		// ErrInvalidSignature, ErrMalformed, and anything else are
		// the same outcome to the recipient: the token can't be
		// trusted, so we refuse without leaking which check failed.
		apierr.WriteError(w, apierr.APIError{
			Status: http.StatusUnauthorized, Code: "invalid_token",
			Message: "unsubscribe token could not be verified",
		})
		return
	}

	tenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		apierr.WriteError(w, apierr.APIError{
			Status: http.StatusUnauthorized, Code: "invalid_token",
			Message: "unsubscribe token carries an invalid tenant id",
		})
		return
	}

	if err := h.recorder.RecordUnsubscribe(r.Context(), claims.Email, &tenantID, suppression.ReasonListUnsubscribe); err != nil {
		apierr.WriteError(w, apierr.APIError{
			Status: http.StatusInternalServerError, Code: "unsubscribe_failed",
			Message: err.Error(),
		})
		return
	}

	writeUnsubscribedHTML(w, claims.Email)
}

// writeUnsubscribedHTML returns a minimal confirmation page. Kept
// tiny on purpose — every CSS rule or extra link is a chance for an
// email client's strict renderer to mangle the response. The address
// the recipient just removed is echoed back so they can tell the
// click was applied to the right inbox.
func writeUnsubscribedHTML(w http.ResponseWriter, email string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Unsubscribed</title></head>
<body style="font-family:system-ui,sans-serif;max-width:480px;margin:48px auto;padding:0 24px;line-height:1.5;color:#222;">
<h1 style="font-size:20px;margin-bottom:8px;">You've been unsubscribed</h1>
<p>%s will no longer receive messages from this campaign or any campaign on this workspace.</p>
</body>
</html>`, htmlEscape(email))
}

// htmlEscape covers the &/</>/" set — enough for echoing back an
// email address. We don't pull in html/template just to render four
// lines of static HTML around one user-controlled string.
func htmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}
