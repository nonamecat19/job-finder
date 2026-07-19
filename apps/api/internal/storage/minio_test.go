package storage

import (
	"context"
	"testing"
	"time"
)

// compile-time proof the adapter satisfies the port.
var _ Blobstore = (*MinioStore)(nil)

// NewMinioStore should surface a connection error (not hang) when the endpoint
// is unreachable, and must not panic on a blank bucket (defaults to documents).
func TestNewMinioStore_UnreachableEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := NewMinioStore(ctx, Config{
		Endpoint:  "127.0.0.1:1", // nothing listens here
		AccessKey: "x",
		SecretKey: "y",
		// Bucket intentionally empty -> defaults to "documents".
	})
	if err == nil {
		t.Fatal("expected error connecting to unreachable MinIO endpoint, got nil")
	}
}
