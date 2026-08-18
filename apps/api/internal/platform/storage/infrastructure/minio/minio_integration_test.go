//go:build integration

package minio

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/job-finder/api/internal/testinfra"
)

func statOptions() minio.StatObjectOptions { return minio.StatObjectOptions{} }

// The unit test in this package only proves an unreachable endpoint errors.
// Everything the application actually relies on — that New creates the bucket
// when it is missing, that an upload survives a round trip byte for byte, and
// that the content type it declares comes back — is behaviour of a real S3
// implementation, so it needs one. These run against the MinIO image
// docker-compose.yml runs.

func newStore(t *testing.T, bucket string) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	endpoint, err := testinfra.MinIOEndpoint(ctx)
	if err != nil {
		t.Fatalf("start minio: %v", err)
	}
	store, err := New(ctx, Config{
		Endpoint:  endpoint,
		AccessKey: testinfra.MinIOAccessKey,
		SecretKey: testinfra.MinIOSecretKey,
		Bucket:    bucket,
		UseSSL:    false,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "document.pdf")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

// TestNewCreatesMissingBucket proves ensureBucket does what the deployment
// relies on. The `createbuckets` compose service makes the bucket too, so a
// test that pre-created it would prove nothing about a fresh volume — this
// one names a bucket nothing has ever made.
func TestNewCreatesMissingBucket(t *testing.T) {
	store := newStore(t, "documents-created-by-the-adapter")

	ctx := context.Background()
	exists, err := store.client.BucketExists(ctx, store.bucket)
	if err != nil {
		t.Fatalf("BucketExists: %v", err)
	}
	if !exists {
		t.Fatal("New returned without creating the bucket it was configured with")
	}
}

// TestNewIsIdempotentOverAnExistingBucket proves a restart against a volume
// that already holds the bucket is not an error — the ordinary case every
// time the API container starts.
func TestNewIsIdempotentOverAnExistingBucket(t *testing.T) {
	const bucket = "documents-reopened"
	newStore(t, bucket)
	newStore(t, bucket)
}

// TestUploadDownloadRoundTrip is the property the generated-document flow
// depends on: what RenderCV wrote to disk is what a later download hands
// back, unchanged, with the content type it was uploaded under.
func TestUploadDownloadRoundTrip(t *testing.T) {
	store := newStore(t, testinfra.MinIOBucket)
	ctx := context.Background()

	const body = "%PDF-1.7\nnot really a pdf, but bytes are bytes\n"
	const key = "generated/roundtrip.pdf"
	if err := store.Upload(ctx, key, writeTemp(t, body), "application/pdf"); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	reader, err := store.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read downloaded object: %v", err)
	}
	if string(got) != body {
		t.Fatalf("downloaded %q, want %q", got, body)
	}

	info, err := store.client.StatObject(ctx, store.bucket, key, statOptions())
	if err != nil {
		t.Fatalf("StatObject: %v", err)
	}
	if info.ContentType != "application/pdf" {
		t.Fatalf("ContentType = %q, want application/pdf: the type is not stored with the object", info.ContentType)
	}
}

// TestUploadOverwritesSameKey proves a re-run of a generation replaces the
// document rather than erroring or leaving the old bytes in place.
func TestUploadOverwritesSameKey(t *testing.T) {
	store := newStore(t, testinfra.MinIOBucket)
	ctx := context.Background()

	const key = "generated/overwritten.pdf"
	if err := store.Upload(ctx, key, writeTemp(t, "first"), "application/pdf"); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if err := store.Upload(ctx, key, writeTemp(t, "second"), "application/pdf"); err != nil {
		t.Fatalf("second upload: %v", err)
	}

	reader, err := store.Download(ctx, key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("downloaded %q, want the second upload's bytes", got)
	}
}

// TestDownloadMissingKeyFails pins where the error surfaces. minio-go's
// GetObject is lazy — it returns an object handle without contacting the
// server — so a caller that only checks Download's error and never reads
// would treat a missing document as success. This documents that the read is
// what fails, which is what callers must therefore do.
func TestDownloadMissingKeyFails(t *testing.T) {
	store := newStore(t, testinfra.MinIOBucket)
	ctx := context.Background()

	reader, err := store.Download(ctx, "generated/never-uploaded.pdf")
	if err != nil {
		return // an eager implementation would be fine too
	}
	defer reader.Close()

	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("reading a key that was never uploaded returned no error")
	} else if !strings.Contains(strings.ToLower(err.Error()), "does not exist") &&
		!strings.Contains(strings.ToLower(err.Error()), "no such key") {
		t.Fatalf("unexpected error for a missing key: %v", err)
	}
}
