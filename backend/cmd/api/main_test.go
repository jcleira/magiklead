package main

import (
	"strings"
	"testing"
)

func TestValidateClerkKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
		errSub  string
	}{
		{
			// Synthetic shapes — these are not real Clerk keys, they
			// only have to satisfy the regex. Avoid plausible
			// alphabets to keep secret scanners quiet.
			name: "valid sk_test_ shape",
			key:  "sk_test_" + strings.Repeat("x", 24),
		},
		{
			name: "valid sk_live_ shape",
			key:  "sk_live_" + strings.Repeat("x", 24),
		},
		{
			name:    "placeholder value",
			key:     "sk_test_YOUR_CLERK_SECRET_KEY_HERE",
			wantErr: true,
			errSub:  "CLERK_SECRET_KEY",
		},
		{
			name:    "empty string",
			key:     "",
			wantErr: true,
			errSub:  "required",
		},
		{
			name:    "malformed prefix",
			key:     "pk_test_" + strings.Repeat("x", 24),
			wantErr: true,
			errSub:  "CLERK_SECRET_KEY",
		},
		{
			name:    "too short",
			key:     "sk_test_" + strings.Repeat("x", 10),
			wantErr: true,
			errSub:  "CLERK_SECRET_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClerkKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
