package nativesymbols

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

type testAuthorizer struct{ err error }

func (a testAuthorizer) Authorize(context.Context, string) error { return a.err }

type testStore struct {
	mu                sync.Mutex
	data              []byte
	readErr, writeErr error
	writes            int
}

func (s *testStore) Get(context.Context, string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		return nil, s.readErr
	}
	if s.data == nil {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(s.data)), nil
}
func (s *testStore) PutIfAbsent(_ context.Context, _, _ string, r io.Reader, _ int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return false, s.writeErr
	}
	if s.data != nil {
		return false, nil
	}
	s.writes++
	s.data, _ = io.ReadAll(r)
	return true, nil
}

func TestUploadRetainsPrivateImmutableSymbols(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		authErr, readErr, writeErr error
		initial, body              string
		want, writes               int
	}{
		{name: "new", body: "valid symbols", want: 204, writes: 1},
		{name: "idempotent retry", initial: "valid symbols", body: "valid symbols", want: 204},
		{name: "replacement", initial: "different symbols", body: "valid symbols", want: 409},
		{name: "invalid identity", body: "wrong UUID", want: 400},
		{name: "untrusted caller", authErr: errors.New("invalid"), body: "valid symbols", want: 401},
		{name: "read outage", initial: "valid symbols", readErr: errors.New("outage"), body: "valid symbols", want: 503},
		{name: "write outage", writeErr: errors.New("outage"), body: "valid symbols", want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{readErr: tc.readErr, writeErr: tc.writeErr}
			if tc.initial != "" {
				store.data = []byte(tc.initial)
			}
			h := NewHandler(store, testAuthorizer{tc.authErr}, func(string, string) (string, error) { return "symbols", nil }, func(data []byte, _, _ string) error {
				if string(data) != "valid symbols" {
					return errors.New("invalid")
				}
				return nil
			})
			r := httptest.NewRequest(http.MethodPut, "/v1/native-symbols/id/arm64", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer signed-ci-token")
			r.Header.Set("Content-Type", "application/octet-stream")
			r.SetPathValue("image_uuid", "id")
			r.SetPathValue("architecture", "arm64")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want || store.writes != tc.writes {
				t.Fatalf("status=%d writes=%d", w.Code, store.writes)
			}
			if strings.Contains(w.Body.String(), tc.body) {
				t.Fatal("echoed private symbol bytes")
			}
		})
	}
}

func TestUploadBoundsBodyBeforeValidation(t *testing.T) {
	store := &testStore{}
	h := NewHandler(store, testAuthorizer{}, func(string, string) (string, error) { return "symbols", nil }, func([]byte, string, string) error { t.Fatal("validated oversized body"); return nil })
	r := httptest.NewRequest(http.MethodPut, "/", io.LimitReader(zeroReader{}, MaxSymbolBytes+1))
	r.Header.Set("Authorization", "Bearer signed-ci-token")
	r.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusRequestEntityTooLarge || store.writes != 0 {
		t.Fatalf("status=%d writes=%d", w.Code, store.writes)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestSeparateHandlersCannotReplaceConcurrentSymbols(t *testing.T) {
	store := &testStore{}
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, body := range []string{"first valid symbols", "second valid symbols"} {
		go func() {
			h := NewHandler(store, testAuthorizer{}, func(string, string) (string, error) { return "same-key", nil }, func([]byte, string, string) error { return nil })
			r := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer signed-ci-token")
			r.Header.Set("Content-Type", "application/octet-stream")
			w := httptest.NewRecorder()
			<-start
			h.ServeHTTP(w, r)
			results <- w.Code
		}()
	}
	close(start)
	statuses := map[int]int{}
	statuses[<-results]++
	statuses[<-results]++
	if statuses[204] != 1 || statuses[409] != 1 || store.writes != 1 {
		t.Fatalf("statuses=%v writes=%d", statuses, store.writes)
	}
}
