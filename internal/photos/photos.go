// Package photos stores plant images in object storage. Postgres holds the key,
// never the bytes.
package photos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// signingRegion is baked in so presigning never has to ask the bucket where it
// lives. The signer may point at a hostname the service itself cannot reach.
const signingRegion = "us-east-1"

// ErrUnavailable reports configured storage that has not connected yet. It is
// distinct from disabled storage so readiness can say whether retrying will
// heal the service without a new deployment.
var ErrUnavailable = errors.New("photo storage is not ready")

// Storage is the object boundary used by the API. Manager implements it while
// reconnecting underneath callers, and Store remains useful directly in tests.
type Storage interface {
	Put(context.Context, string, string, io.Reader, int64) (string, error)
	Get(context.Context, string) (io.ReadCloser, error)
	URL(context.Context, string, time.Duration) (string, error)
	Delete(context.Context, string) error
}

// State is the readiness of configured photograph storage.
type State string

const (
	StateStarting    State = "starting"
	StateReady       State = "ready"
	StateUnavailable State = "unavailable"
)

// Store is an S3-compatible bucket, in practice the MinIO already running here.
type Store struct {
	client *minio.Client
	signer *minio.Client
	bucket string
}

// Config is what it takes to reach the bucket. PublicEndpoint is separate
// because a link signed against the pod's cluster DNS name is unusable to a
// phone, and the signed host cannot be swapped afterwards.
type Config struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
	PublicSSL      bool
}

// Manager keeps a configured store recoverable. The API holds this stable
// object while the backing Store is atomically replaced after a successful
// retry, so routes recover without rebuilding the mux or restarting the pod.
type Manager struct {
	current atomic.Pointer[Store]

	mu      sync.RWMutex
	state   State
	lastErr error

	open     func(context.Context, Config) (*Store, error)
	config   Config
	retryMin time.Duration
	retryMax time.Duration
	changed  func(State, error)
}

// Manage starts connecting immediately and retries forever with a capped
// exponential backoff. The retry interval is bounded; availability is not.
func Manage(ctx context.Context, cfg Config, changed func(State, error)) *Manager {
	m := &Manager{
		state:    StateStarting,
		open:     Open,
		config:   cfg,
		retryMin: time.Second,
		retryMax: 30 * time.Second,
		changed:  changed,
	}
	go m.run(ctx)
	return m
}

func (m *Manager) run(ctx context.Context) {
	delay := m.retryMin
	for {
		store, err := m.open(ctx, m.config)
		if err == nil {
			m.current.Store(store)
			m.setState(StateReady, nil)
			return
		}
		m.setState(StateUnavailable, err)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if delay < m.retryMax {
			delay *= 2
			if delay > m.retryMax {
				delay = m.retryMax
			}
		}
	}
}

func (m *Manager) setState(state State, err error) {
	m.mu.Lock()
	changed := m.state != state
	m.state, m.lastErr = state, err
	m.mu.Unlock()
	if changed && m.changed != nil {
		m.changed(state, err)
	}
}

// Status reports readiness without exposing credentials or upstream detail.
// LastError is retained for logs and tests, not returned from the HTTP API.
func (m *Manager) Status() (State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state, m.lastErr
}

func (m *Manager) store() (*Store, error) {
	store := m.current.Load()
	if store == nil {
		return nil, ErrUnavailable
	}
	return store, nil
}

func (m *Manager) Put(ctx context.Context, key, contentType string, body io.Reader, size int64) (string, error) {
	store, err := m.store()
	if err != nil {
		return "", err
	}
	return store.Put(ctx, key, contentType, body, size)
}

func (m *Manager) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	store, err := m.store()
	if err != nil {
		return nil, err
	}
	return store.Get(ctx, key)
}

func (m *Manager) URL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	store, err := m.store()
	if err != nil {
		return "", err
	}
	return store.URL(ctx, key, ttl)
}

func (m *Manager) Delete(ctx context.Context, key string) error {
	store, err := m.store()
	if err != nil {
		return err
	}
	return store.Delete(ctx, key)
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

	signer, err := signingClient(cfg, client)
	if err != nil {
		return nil, err
	}
	return &Store{client: client, signer: signer, bucket: cfg.Bucket}, nil
}

// signingClient returns the client that presigns links, which is the reaching
// client unless a separate public endpoint says otherwise. It is never dialled,
// so its host only has to resolve for whoever follows the link.
func signingClient(cfg Config, reaching *minio.Client) (*minio.Client, error) {
	if cfg.PublicEndpoint == "" || cfg.PublicEndpoint == cfg.Endpoint {
		return reaching, nil
	}
	signer, err := minio.New(cfg.PublicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.PublicSSL,
		Region: signingRegion,
	})
	if err != nil {
		return nil, fmt.Errorf("connect public endpoint: %w", err)
	}
	return signer, nil
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
	u, err := s.signer.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", fmt.Errorf("sign %s: %w", key, err)
	}
	return u.String(), nil
}

// Delete removes bytes after a failed database write or an explicit retention
// action. A missing key is success, which makes compensation safe to retry.
func (s *Store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}
