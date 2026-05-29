// Package jwt is a minimal HS256 JWT encoder/decoder used by the
// unsubscribe one-click flow (issue #6) and the Gmail OAuth connect
// flow (issue #3/#8). Token bodies are small (a couple of ids +
// expiry), the only algorithm in use is HS256,
// and the only verification logic is "signature matches and exp is in
// the future" — so we avoid pulling in a full JWT library and keep
// the implementation auditable in one short file.
//
// The encoded form is the RFC 7519 standard:
//
//	base64url(header) "." base64url(payload) "." base64url(signature)
//
// where header is the literal {"alg":"HS256","typ":"JWT"} and the
// signature is HMAC-SHA256(secret, header + "." + payload).
package jwt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors. Callers branch on these to pick HTTP status codes
// (e.g. 401 for tampered vs 410 for expired in the unsubscribe flow).
var (
	ErrMalformed        = errors.New("jwt: malformed token")
	ErrInvalidSignature = errors.New("jwt: invalid signature")
	ErrExpired          = errors.New("jwt: token expired")
)

// Claims is the payload encoded into the JWT. Which fields are set
// depends on the use case: the unsubscribe flow (issue #6) sets Email
// + TenantID for the suppression write; the Gmail OAuth connect flow
// (issue #3/#8) sets TenantID + UserID to bind the public callback to
// the user who started the flow. Exp is always enforced. Unused fields
// are omitted from the wire form so each token carries only what it
// means.
type Claims struct {
	Email    string `json:"email,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	Exp      int64  `json:"exp"`
}

// Encode returns a signed HS256 JWT carrying the given claims.
func Encode(claims Claims, secret []byte) (string, error) {
	headerJSON := []byte(`{"alg":"HS256","typ":"JWT"}`)
	header := base64.RawURLEncoding.EncodeToString(headerJSON)

	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("jwt: marshal claims: %w", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := header + "." + payload
	sig := sign(signingInput, secret)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Decode verifies the signature and expiry, then returns the claims.
// Returns ErrMalformed on parse errors, ErrInvalidSignature on
// signature mismatch, ErrExpired on a past `exp`. Callers must
// type-check the error with errors.Is.
func Decode(token string, secret []byte) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}
	header, payload, signature := parts[0], parts[1], parts[2]

	sigBytes, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: signature decode: %v", ErrMalformed, err)
	}
	expected := sign(header+"."+payload, secret)
	if !hmac.Equal(sigBytes, expected) {
		return Claims{}, ErrInvalidSignature
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: payload decode: %v", ErrMalformed, err)
	}
	var claims Claims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return Claims{}, fmt.Errorf("%w: payload json: %v", ErrMalformed, err)
	}
	if claims.Exp == 0 {
		return Claims{}, fmt.Errorf("%w: missing exp", ErrMalformed)
	}
	if time.Now().Unix() >= claims.Exp {
		return claims, ErrExpired
	}
	return claims, nil
}

func sign(input string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(input))
	return mac.Sum(nil)
}
