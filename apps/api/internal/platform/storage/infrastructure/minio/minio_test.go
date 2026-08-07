package minio

import (
	"context"
	"testing"
	"time"

	"github.com/job-finder/api/internal/platform/storage/domain"
)

var _ domain.Blobstore = (*Store)(nil)

func TestNew_UnreachableEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := New(ctx, Config{
		Endpoint:  "127.0.0.1:1",
		AccessKey: "x",
		SecretKey: "y",
	})
	if err == nil {
		t.Fatal("expected error connecting to unreachable MinIO endpoint, got nil")
	}
}
