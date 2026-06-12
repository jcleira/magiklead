package storage

import "testing"

// parseS3Endpoint strips the scheme that devpods injects into
// S3_ENDPOINT (the MinIO Go SDK's New() only accepts host:port and
// derives TLS from the Secure option). http:// forces useSSL=false,
// https:// forces useSSL=true, and a scheme-less value passes through
// with the caller's default. Garbage stays in the host string so the
// downstream minio.New() call is what surfaces the error.
func TestParseS3Endpoint(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		defaultUseSSL bool
		wantHost      string
		wantUseSSL    bool
	}{
		{
			name:          "http scheme forces plaintext",
			raw:           "http://minio:9000",
			defaultUseSSL: true,
			wantHost:      "minio:9000",
			wantUseSSL:    false,
		},
		{
			name:          "https scheme forces TLS",
			raw:           "https://s3.amazonaws.com",
			defaultUseSSL: false,
			wantHost:      "s3.amazonaws.com",
			wantUseSSL:    true,
		},
		{
			name:          "no scheme preserves default useSSL=false",
			raw:           "minio:9000",
			defaultUseSSL: false,
			wantHost:      "minio:9000",
			wantUseSSL:    false,
		},
		{
			name:          "no scheme preserves default useSSL=true",
			raw:           "s3.amazonaws.com",
			defaultUseSSL: true,
			wantHost:      "s3.amazonaws.com",
			wantUseSSL:    true,
		},
		{
			name:          "malformed garbage passes through unchanged",
			raw:           "::not-a-valid-url::garbage",
			defaultUseSSL: false,
			wantHost:      "::not-a-valid-url::garbage",
			wantUseSSL:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, useSSL := parseS3Endpoint(tt.raw, tt.defaultUseSSL)
			if host != tt.wantHost {
				t.Errorf("host: got %q want %q", host, tt.wantHost)
			}
			if useSSL != tt.wantUseSSL {
				t.Errorf("useSSL: got %v want %v", useSSL, tt.wantUseSSL)
			}
		})
	}
}
