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
