package domain

import (
	"context"
	"io"
)

type Blobstore interface {
	Upload(ctx context.Context, objectKey, localPath, contentType string) error
	Download(ctx context.Context, objectKey string) (io.ReadCloser, error)
}
