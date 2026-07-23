// Package storage is the facade for the object-storage platform kernel: the
// Blobstore port lives in domain/, the MinIO adapter in
// infrastructure/minio/. This file re-exports the shape callers already
// depend on (generation's renderers, cmd/server) so relocating the package
// into internal/platform/storage required no changes at call sites beyond
// the import path.
package storage

import (
	"github.com/job-finder/api/internal/platform/storage/domain"
	"github.com/job-finder/api/internal/platform/storage/infrastructure/minio"
)

type (
	Blobstore  = domain.Blobstore
	Config     = minio.Config
	MinioStore = minio.Store
)

var NewMinioStore = minio.New
