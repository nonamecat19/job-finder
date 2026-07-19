// Package storage is the outbound object-storage port for generated documents.
// Renderers write resume/cover-letter PDFs (and their RenderCV YAML) to the
// local DOCUMENTS_DIR first, then push every file here so all resume files are
// durably stored in object storage (MinIO / S3-compatible).
package storage

import (
	"context"
	"io"
)

// Blobstore is the port the generation renderers depend on. *MinioStore
// satisfies it. When a renderer's store is nil, uploads are skipped and files
// live only on the local filesystem (e.g. in tests or a MinIO-less dev setup).
type Blobstore interface {
	// Upload stores the file at localPath under objectKey. contentType is the
	// MIME type recorded on the object (e.g. "application/pdf").
	Upload(ctx context.Context, objectKey, localPath, contentType string) error
	// Download streams the stored object back. The caller closes the reader.
	Download(ctx context.Context, objectKey string) (io.ReadCloser, error)
}
