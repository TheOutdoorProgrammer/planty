package photos

import (
	"context"
	"errors"
	"io"

	"github.com/minio/minio-go/v7"
)

func (m *Manager) PutIfAbsent(ctx context.Context, key, contentType string, body io.Reader, size int64) (bool, error) {
	store, err := m.store()
	if err != nil {
		return false, err
	}
	return store.PutIfAbsent(ctx, key, contentType, body, size)
}

func (s *Store) PutIfAbsent(ctx context.Context, key, contentType string, body io.Reader, size int64) (bool, error) {
	// A single PUT applies the create-only precondition atomically.
	options := minio.PutObjectOptions{ContentType: contentType, DisableMultipart: true}
	options.SetMatchETagExcept("*")
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, options)
	if err == nil {
		return true, nil
	}
	var response minio.ErrorResponse
	if errors.As(err, &response) && response.Code == "PreconditionFailed" {
		return false, nil
	}
	return false, err
}
