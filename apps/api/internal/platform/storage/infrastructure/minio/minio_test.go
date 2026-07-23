package minio

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/platform/storage/domain"
)

// compile-time proof the adapter satisfies the port.
var _ domain.Blobstore = (*Store)(nil)

// New should surface a connection error (not hang) when the endpoint is
// unreachable, and must not panic on a blank bucket (defaults to documents).
func TestNew_UnreachableEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := New(ctx, Config{
		Endpoint:  "127.0.0.1:1", // nothing listens here
		AccessKey: "x",
		SecretKey: "y",
		// Bucket intentionally empty -> defaults to "documents".
	})
	if err == nil {
		t.Fatal("expected error connecting to unreachable MinIO endpoint, got nil")
	}
}
