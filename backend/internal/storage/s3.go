// Package storage persists raw source payloads for the lead-database
// ingestion pipeline. Every CSV dump, XML filing, and scraped HTML page we
// ingest is uploaded here exactly as received so it can be replayed,
// re-parsed, or audited later. See the "raw ingests preserved forever"
// principle in docs/2026-04-21-lead-database-architecture/research.md.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// RawStorage is the contract every ingest source uses to archive its
// payloads. Backed by any S3-compatible object store — MinIO in dev,
// Hetzner Object Storage or AWS S3 in prod.
type RawStorage interface {
	Upload(ctx context.Context, sourceSlug, fileName string, data io.Reader) (url string, err error)
	Download(ctx context.Context, url string) (io.ReadCloser, error)
	Checksum(ctx context.Context, url string) (string, error)
}

// checksumMetaKey is the user-metadata key that carries the sha256 hex
// digest. Object stores prefix this with "x-amz-meta-" on the wire.
const checksumMetaKey = "sha256"

// S3Storage implements RawStorage against any S3-compatible endpoint.
type S3Storage struct {
	client *minio.Client
	bucket string
}

// NewS3Storage constructs an S3Storage from S3_* env vars. It also
// creates the bucket if missing — convenient for an empty MinIO in dev
// and a no-op in prod where the bucket already exists (or where the IAM
// user lacks s3:CreateBucket, in which case the error surfaces here
// instead of on the first Upload).
func NewS3Storage(ctx context.Context) (*S3Storage, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	accessKey := os.Getenv("S3_ACCESS_KEY")
	secretKey := os.Getenv("S3_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	region := os.Getenv("S3_REGION")
	// Defaults to true; set S3_USE_SSL=false for plaintext MinIO.
	useSSL := !strings.EqualFold(os.Getenv("S3_USE_SSL"), "false")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, errors.New("storage: S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET are required")
	}

	// devpods injects S3_ENDPOINT with an http(s):// scheme; minio.New
	// expects host:port and derives TLS from the Secure option.
	if rest, ok := strings.CutPrefix(endpoint, "https://"); ok {
		endpoint, useSSL = rest, true
	} else if rest, ok := strings.CutPrefix(endpoint, "http://"); ok {
		endpoint, useSSL = rest, false
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: minio client: %w", err)
	}

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: bucket exists check: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: region}); err != nil {
			return nil, fmt.Errorf("storage: create bucket %q: %w", bucket, err)
		}
	}

	return &S3Storage{client: client, bucket: bucket}, nil
}

// Upload stores data under <sourceSlug>/<YYYY-MM-DD>/<fileName> and
// returns an s3://<bucket>/<key> URL. The SHA-256 of the payload is
// attached as user metadata so Checksum can be answered with a single HEAD.
func (s *S3Storage) Upload(ctx context.Context, sourceSlug, fileName string, data io.Reader) (string, error) {
	if sourceSlug == "" || fileName == "" {
		return "", errors.New("storage: sourceSlug and fileName are required")
	}

	// Buffer so we can both hash and upload. Ingest payloads are expected
	// to be CSV dumps and API responses (small-to-medium). A streaming
	// variant is on the list once Common Crawl WARC ingestion lands — see
	// T09 in docs/2026-04-21-lead-database-architecture/plan.md.
	buf, err := io.ReadAll(data)
	if err != nil {
		return "", fmt.Errorf("storage: read input: %w", err)
	}
	sum := sha256.Sum256(buf)
	checksum := hex.EncodeToString(sum[:])

	key := path.Join(sourceSlug, time.Now().UTC().Format("2006-01-02"), fileName)
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err = s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(buf), int64(len(buf)),
		minio.PutObjectOptions{
			ContentType:  contentType,
			UserMetadata: map[string]string{checksumMetaKey: checksum},
		})
	if err != nil {
		return "", fmt.Errorf("storage: put object %q: %w", key, err)
	}

	return fmt.Sprintf("s3://%s/%s", s.bucket, key), nil
}

// Download returns a reader for the object at url. Stat runs first so
// NoSuchKey surfaces here instead of on the first Read.
func (s *S3Storage) Download(ctx context.Context, url string) (io.ReadCloser, error) {
	key, err := s.parseURL(url)
	if err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: get object %q: %w", key, err)
	}
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		return nil, fmt.Errorf("storage: stat object %q: %w", key, err)
	}
	return obj, nil
}

// Checksum returns the sha256 hex digest stored as object metadata at
// upload time.
func (s *S3Storage) Checksum(ctx context.Context, url string) (string, error) {
	key, err := s.parseURL(url)
	if err != nil {
		return "", err
	}
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("storage: stat object %q: %w", key, err)
	}
	// UserMetadata key casing differs across S3 providers, so compare
	// case-insensitively. Fall back to the raw response header if the
	// parsed map is empty (some providers only expose it there).
	for k, v := range info.UserMetadata {
		if strings.EqualFold(k, checksumMetaKey) {
			return v, nil
		}
	}
	if v := info.Metadata.Get("X-Amz-Meta-" + checksumMetaKey); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("storage: no %s metadata on %s", checksumMetaKey, url)
}

// parseURL accepts s3://<bucket>/<key> and returns the key after
// verifying the bucket matches the configured one.
func (s *S3Storage) parseURL(raw string) (string, error) {
	const scheme = "s3://"
	if !strings.HasPrefix(raw, scheme) {
		return "", fmt.Errorf("storage: not an s3 url: %q", raw)
	}
	rest := raw[len(scheme):]
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return "", fmt.Errorf("storage: malformed s3 url: %q", raw)
	}
	bucket, key := rest[:slash], rest[slash+1:]
	if bucket != s.bucket {
		return "", fmt.Errorf("storage: bucket mismatch: url=%s, configured=%s", bucket, s.bucket)
	}
	return key, nil
}
