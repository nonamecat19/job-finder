// Package minio implements domain.Blobstore against a MinIO/S3-compatible
// object store.
package minio

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config holds the MinIO/S3 connection settings, mirroring the MINIO_* env keys.
type Config struct {
	Endpoint  string // host:port, e.g. "minio:9000" (no scheme)
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Store is the MinIO-backed domain.Blobstore adapter.
type Store struct {
	client *minio.Client
	bucket string
}

// New connects to MinIO and ensures the target bucket exists.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Bucket == "" {
		cfg.Bucket = "documents"
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: minio client: %w", err)
	}
	s := &Store{client: client, bucket: cfg.Bucket}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// ensureBucket creates the bucket on first use so a fresh MinIO needs no manual
// setup (the compose createbuckets init handles the same for the CLI path).
func (s *Store) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("storage: bucket check %q: %w", s.bucket, err)
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("storage: make bucket %q: %w", s.bucket, err)
		}
	}
	return nil
}

// Upload puts the local file into the bucket under objectKey.
func (s *Store) Upload(ctx context.Context, objectKey, localPath, contentType string) error {
	if _, err := s.client.FPutObject(ctx, s.bucket, objectKey, localPath, minio.PutObjectOptions{ContentType: contentType}); err != nil {
		return fmt.Errorf("storage: upload %q: %w", objectKey, err)
	}
	return nil
}

// Download streams the object back. The returned reader must be closed.
func (s *Store) Download(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage: download %q: %w", objectKey, err)
	}
	return obj, nil
}
