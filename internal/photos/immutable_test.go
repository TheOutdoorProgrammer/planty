package photos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestPutIfAbsentUsesAtomicStoragePrecondition(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "created", true: "exists"}[exists], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != "/private/symbols" || r.Header.Get("If-None-Match") != "*" || r.URL.RawQuery != "" {
					t.Errorf("expected conditional single PUT: %s %s condition=%q", r.Method, r.URL.Path, r.Header.Get("If-None-Match"))
				}
				if exists {
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusPreconditionFailed)
					_, _ = w.Write([]byte(`<Error><Code>PreconditionFailed</Code><Message>Already exists</Message></Error>`))
					return
				}
				w.Header().Set("ETag", `"fixture"`)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{Creds: credentials.NewStaticV4("fixture-access", "fixture-secret", ""), Region: "us-east-1"})
			if err != nil {
				t.Fatal(err)
			}
			store := &Store{client: client, bucket: "private"}
			created, err := store.PutIfAbsent(context.Background(), "symbols", "application/octet-stream", strings.NewReader("data"), 4)
			if err != nil || created == exists {
				t.Fatalf("created=%v error=%v", created, err)
			}
		})
	}
}
