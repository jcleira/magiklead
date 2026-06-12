package jwt

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// secret is shared across these tests — it's not a real production
// secret; the package is unit-tested without env vars.
var secret = []byte("test-secret-do-not-use-in-prod")

// Tracer: encode → decode round-trips claims verbatim.
func TestEncodeDecode_RoundTrip(t *testing.T) {
	claims := Claims{
		Email:    "lead@example.com",
		TenantID: "00000000-0000-0000-0000-000000000001",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	tok, err := Encode(claims, secret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(tok, secret)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != claims {
		t.Errorf("claims=%+v want %+v", got, claims)
	}
}

// The Gmail OAuth connect flow signs TenantID + UserID (no email) into
// the state; that combination must round-trip verbatim too.
func TestEncodeDecode_GmailStateClaims(t *testing.T) {
	claims := Claims{
		TenantID: "00000000-0000-0000-0000-000000000001",
		UserID:   "00000000-0000-0000-0000-0000000000ab",
		Exp:      time.Now().Add(15 * time.Minute).Unix(),
	}
	tok, err := Encode(claims, secret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(tok, secret)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != claims {
		t.Errorf("claims=%+v want %+v", got, claims)
	}
	if got.Email != "" {
		t.Errorf("email=%q want empty for a gmail-state token", got.Email)
	}
}

// A token signed with a different secret must return ErrInvalidSignature.
func TestDecode_TamperedSignature(t *testing.T) {
	claims := Claims{
		Email:    "lead@example.com",
		TenantID: "00000000-0000-0000-0000-000000000001",
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	tok, err := Encode(claims, []byte("wrong-secret"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, err = Decode(tok, secret)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("err=%v want ErrInvalidSignature", err)
	}
}

// Mutating the payload after signing must trip the signature check.
func TestDecode_TamperedPayload(t *testing.T) {
	claims := Claims{Email: "a@b.com", TenantID: "t", Exp: time.Now().Add(time.Hour).Unix()}
	tok, err := Encode(claims, secret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Replace one character in the payload segment.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token shape unexpected: %q", tok)
	}
	parts[1] = parts[1][:len(parts[1])-2] + "AA"
	tampered := strings.Join(parts, ".")

	_, err = Decode(tampered, secret)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("err=%v want ErrInvalidSignature", err)
	}
}

// An exp in the past must return ErrExpired.
func TestDecode_Expired(t *testing.T) {
	claims := Claims{
		Email:    "lead@example.com",
		TenantID: "tenant",
		Exp:      time.Now().Add(-1 * time.Minute).Unix(),
	}
	tok, err := Encode(claims, secret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, err = Decode(tok, secret)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("err=%v want ErrExpired", err)
	}
}

// Malformed inputs: wrong segment count, garbage in the signature
// segment, missing exp. All must fail closed.
func TestDecode_Malformed(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"one_segment":   "abc",
		"two_segments":  "abc.def",
		"four_segments": "a.b.c.d",
		"bad_signature": "eyJhbGciOiJIUzI1NiJ9.eyJleHAiOjF9.!!!not-base64!!!",
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(tok, secret)
			if err == nil {
				t.Errorf("expected error for %q", tok)
			}
		})
	}
}

// Missing exp claim: even with a valid signature, a token with no
// expiry is rejected as malformed so a misconfigured caller can't
// accidentally issue immortal tokens.
func TestDecode_MissingExp(t *testing.T) {
	// Manually build a token whose payload omits exp.
	tok, err := Encode(Claims{Email: "a@b.com", TenantID: "t"}, secret)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, err = Decode(tok, secret)
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("err=%v want ErrMalformed", err)
	}
}
