package nativesymbols

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const MaxSymbolBytes = 64 << 20

type Store interface {
	PutIfAbsent(context.Context, string, string, io.Reader, int64) (bool, error)
	Get(context.Context, string) (io.ReadCloser, error)
}

type Handler struct {
	store      Store
	authorizer Authorizer
	key        func(string, string) (string, error)
	validate   func([]byte, string, string) error
	inflight   chan struct{}
}

func NewHandler(store Store, authorizer Authorizer, key func(string, string) (string, error), validate func([]byte, string, string) error) *Handler {
	return &Handler{store: store, authorizer: authorizer, key: key, validate: validate, inflight: make(chan struct{}, 1)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodPut)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" || h.authorizer.Authorize(r.Context(), token) != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id, arch := r.PathValue("image_uuid"), r.PathValue("architecture")
	key, err := h.key(id, arch)
	if err != nil {
		http.Error(w, "invalid symbol identity", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		http.Error(w, "expected DWARF object", http.StatusUnsupportedMediaType)
		return
	}
	select {
	case h.inflight <- struct{}{}:
		defer func() { <-h.inflight }()
	default:
		w.Header().Set("Retry-After", "30")
		http.Error(w, "symbol upload busy", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(time.Minute))
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	r.Body = http.MaxBytesReader(w, r.Body, MaxSymbolBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			http.Error(w, "symbol object too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid symbol body", http.StatusBadRequest)
		}
		return
	}
	if h.validate(data, id, arch) != nil {
		http.Error(w, "symbol object does not match identity", http.StatusBadRequest)
		return
	}
	created, err := h.store.PutIfAbsent(r.Context(), key, "application/octet-stream", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		http.Error(w, "symbol storage unavailable", http.StatusServiceUnavailable)
		return
	}
	if created {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	existing, err := h.store.Get(r.Context(), key)
	if err != nil {
		http.Error(w, "symbol storage unavailable", http.StatusServiceUnavailable)
		return
	}
	stored, readErr := io.ReadAll(io.LimitReader(existing, MaxSymbolBytes+1))
	closeErr := existing.Close()
	if readErr != nil || closeErr != nil {
		http.Error(w, "symbol storage unavailable", http.StatusServiceUnavailable)
		return
	}
	if len(stored) > MaxSymbolBytes || sha256.Sum256(stored) != sha256.Sum256(data) {
		http.Error(w, "symbol identity already exists", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
