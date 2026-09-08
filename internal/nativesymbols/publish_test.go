package nativesymbols

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublisherUsesDistinctIdentityAndRequiresDurableAcknowledgement(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var uploads int
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/identity":
					if r.Header.Get("Authorization") != "Bearer request-secret" || r.URL.Query().Get("audience") != Audience {
						t.Error("wrong identity request")
					}
					_, _ = io.WriteString(w, `{"value":"workload-token"}`)
				case "/v1/native-symbols/image/arm64":
					uploads++
					if r.Method != http.MethodPut || r.Header.Get("Authorization") != "Bearer workload-token" {
						t.Error("wrong upload identity")
					}
					body, _ := io.ReadAll(r.Body)
					if string(body) != "private dwarf" {
						t.Error("wrong symbol body")
					}
					w.WriteHeader(status)
					_, _ = io.WriteString(w, "private upstream error with token")
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			err := publish(context.Background(), server.Client(), server.URL, server.URL+"/identity", "request-secret", "image", "arm64", []byte("private dwarf"))
			if (err == nil) != (status == http.StatusNoContent) || uploads != 1 {
				t.Fatalf("error=%v uploads=%d", err, uploads)
			}
			if err != nil && (strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "token")) {
				t.Fatal("leaked response")
			}
		})
	}
}
