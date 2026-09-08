package nativetelemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/photos"
)

type observedSymbolStore struct {
	ObjectStore
	keys               []string
	readers            []*observedSymbolReader
	get                func(context.Context, string) (io.ReadCloser, error)
	overlappingReaders bool
}

func (s *observedSymbolStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if len(s.readers) > 0 && !s.readers[len(s.readers)-1].closed {
		s.overlappingReaders = true
	}
	s.keys = append(s.keys, key)
	var reader io.ReadCloser
	var err error
	if s.get != nil {
		reader, err = s.get(ctx, key)
	} else {
		reader, err = s.ObjectStore.Get(ctx, key)
	}
	if err != nil {
		return nil, err
	}
	observed := &observedSymbolReader{ReadCloser: reader}
	s.readers = append(s.readers, observed)
	return observed, nil
}

type observedSymbolReader struct {
	io.ReadCloser
	closed bool
	read   int
}

func (r *observedSymbolReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.read += n
	return n, err
}

func (r *observedSymbolReader) Close() error {
	r.closed = true
	return r.ReadCloser.Close()
}

func minioSymbolStore(t *testing.T, objectHandler http.HandlerFunc) *observedSymbolStore {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Has("location") {
			_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
			return
		}
		if r.Method == http.MethodHead && r.URL.Path == "/private/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		objectHandler(w, r)
	}))
	t.Cleanup(server.Close)
	store, err := photos.Open(context.Background(), photos.Config{
		Endpoint: strings.TrimPrefix(server.URL, "http://"), Bucket: "private",
		AccessKey: "fixture-access", SecretKey: "fixture-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &observedSymbolStore{ObjectStore: store}
}

func missingSymbolObject(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>fixture missing</Message></Error>`)
}

func TestMinIOLazySymbolFallback(t *testing.T) {
	data := fixtureSymbols(t)
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("mismatched_uuid=%t", mismatch), func(t *testing.T) {
			id, command := fixtureUUID, "must-never-run"
			if mismatch {
				id = "8d6c4d58-85dc-4b1f-bd26-8d20319b646c"
			} else {
				command = testSymbolizerCommand(t)
			}
			primary, _ := SymbolKey(id, "arm64e")
			fallback, _ := SymbolKey(id, "arm64")
			store := minioSymbolStore(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/private/" + primary:
					missingSymbolObject(w)
				case "/private/" + fallback:
					w.Header().Set("Content-Length", fmt.Sprint(len(data)))
					w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
					_, _ = w.Write(data)
				default:
					t.Errorf("unexpected object request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
				}
			})
			// GetObject succeeds before the server has returned NoSuchKey.
			reader, err := store.ObjectStore.Get(context.Background(), primary)
			if err != nil {
				t.Fatalf("expected lazy GetObject, got %v", err)
			}
			_, readErr := io.ReadAll(reader)
			_ = reader.Close()
			if readErr == nil {
				t.Fatal("missing object did not fail during Read")
			}
			symbolizer := &Symbolizer{Store: store, Command: command}
			frames, err := symbolizer.Resolve(context.Background(), Crash{ImageUUID: id, Architecture: "arm64e", Frames: []Frame{{Offset: 0x328}}})
			if mismatch {
				if !errors.Is(err, ErrInvalidSymbols) {
					t.Fatalf("fallback must reject another image: %v", err)
				}
			} else if err != nil || !slices.Equal(frames, []ResolvedFrame{{Function: "native_crash_site", File: "symbols.c", Line: 1}}) {
				t.Fatalf("lazy fallback did not resolve the exact app image: %#v %v", frames, err)
			}
			if !slices.Equal(store.keys, []string{primary, fallback}) {
				t.Fatalf("candidate lookup order = %v", store.keys)
			}
			if store.overlappingReaders {
				t.Fatal("fallback opened before closing failed candidate")
			}
			for _, reader := range store.readers {
				if !reader.closed {
					t.Fatal("candidate reader leaked")
				}
			}
		})
	}
}

func TestMinIOSymbolReadCancellationStopsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := minioSymbolStore(t, func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		missingSymbolObject(w)
	})
	symbolizer := &Symbolizer{Store: store, Command: "must-never-run"}
	_, err := symbolizer.Resolve(ctx, Crash{ImageUUID: fixtureUUID, Architecture: "arm64e", Frames: []Frame{{Offset: 0x328}}})
	if err == nil || ctx.Err() != context.Canceled || len(store.keys) != 1 {
		t.Fatalf("cancelled lookup attempted fallback: %v %v", store.keys, err)
	}
	if len(store.readers) != 1 || !store.readers[0].closed {
		t.Fatal("cancelled candidate reader leaked")
	}
}

type zeroSymbolReader struct{}

func (zeroSymbolReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestSymbolReadBoundsFailClosed(t *testing.T) {
	for name, reader := range map[string]io.Reader{
		"oversize":           zeroSymbolReader{},
		"wrong architecture": strings.NewReader(string(fixtureSymbols(t))),
	} {
		t.Run(name, func(t *testing.T) {
			store := &observedSymbolStore{get: func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(reader), nil
			}}
			symbolizer := &Symbolizer{Store: store, Command: "must-never-run"}
			_, err := symbolizer.Resolve(context.Background(), Crash{ImageUUID: fixtureUUID, Architecture: "arm64e", Frames: []Frame{{Offset: 0x328}}})
			if !errors.Is(err, ErrInvalidSymbols) || len(store.keys) != 1 {
				t.Fatalf("invalid candidate attempted fallback: %v %v", store.keys, err)
			}
			if len(store.readers) != 1 || !store.readers[0].closed {
				t.Fatal("invalid candidate reader leaked")
			}
			if name == "oversize" && store.readers[0].read != MaxSymbolBytes+1 {
				t.Fatalf("read escaped size bound: %d", store.readers[0].read)
			}
		})
	}
}
