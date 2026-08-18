// Package photos stores plant images in object storage. Postgres holds the key,
// never the bytes.
package photos

import (
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Store is an S3-compatible bucket, in practice the MinIO already running here.
type Store struct {
	client *minio.Client
	bucket string
}

// Config is what it takes to reach the bucket.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// Open connects and ensures the bucket exists.
func Open(ctx context.Context, cfg Config) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("connect object storage: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket: %w", err)
		}
	}
	return &Store{client: client, bucket: cfg.Bucket}, nil
}

// Key lays photos out by plant and date so a prefix listing is a timeline.
func Key(plantSlug string, takenAt time.Time, ext string) string {
	return path.Join(plantSlug, takenAt.UTC().Format("2006/01/02"),
		uuid.NewString()+ext)
}

// Put uploads an image and returns its storage key.
func (s *Store) Put(ctx context.Context, key, contentType string, body io.Reader, size int64) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", key, err)
	}
	return key, nil
}

// Get returns the stored bytes. The caller closes the reader.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", key, err)
	}
	return obj, nil
}

// URL returns a short-lived link, so the app can render an image without the
// service proxying every byte.
func (s *Store) URL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("sign %s: %w", key, err)
	}
	return u.String(), nil
}
