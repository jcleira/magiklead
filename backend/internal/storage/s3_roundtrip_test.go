//go:build integration

package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
)

// Round-trips an upload → download → checksum against a live S3 endpoint.
// Run with `go test -tags=integration ./internal/storage/...` after env
// vars are exported (see .env.example). Skipped by default so normal
// `go test ./...` doesn't require MinIO.
func TestRoundTripIntegration(t *testing.T) {
	if os.Getenv("S3_ENDPOINT") == "" {
		t.Skip("S3_ENDPOINT not set; skipping integration test")
	}
	ctx := context.Background()

	s, err := NewS3Storage(ctx)
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}

	payload := []byte("hello magiklead raw ingest")
	expected := sha256.Sum256(payload)
	expectedHex := hex.EncodeToString(expected[:])

	url, err := s.Upload(ctx, "test-source", "hello.txt", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	t.Logf("uploaded to %s", url)

	rc, err := s.Download(ctx, url)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %q want %q", got, payload)
	}

	checksum, err := s.Checksum(ctx, url)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if checksum != expectedHex {
		t.Fatalf("checksum mismatch: got %s want %s", checksum, expectedHex)
	}
}
