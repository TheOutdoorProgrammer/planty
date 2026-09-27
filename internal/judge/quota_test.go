package judge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenCodeQuotaIsDistinctFromTransientRateLimits(t *testing.T) {
	for _, tc := range []struct {
		name, provider, message string
		status                  int
		quota                   bool
	}{
		{"quota", "opencode-go", "Go usage limit exceeded", 429, true},
		{"rate limit", "opencode-go", "Too many requests", 429, false},
		{"server failure", "opencode-go", "Go usage limit exceeded", 503, false},
		{"other provider", "other", "Go usage limit exceeded", 429, false},
		{"unrecognized message", "opencode-go", "Go usage limit exceeded: private detail", 429, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 1 {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"recovered"}}]}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprintf(w, `{"error":{"message":%q},"private":"must not leak"}`, tc.message)
			}))
			defer server.Close()
			backend := newOpenAIBackend(Provider{ID: tc.provider, BaseURL: server.URL}, "test")
			_, err := backend.call(context.Background(), chatRequest{})
			if err == nil || errors.Is(err, ErrQuotaExhausted) != tc.quota || Retryable(err) == tc.quota {
				t.Fatalf("quota=%t retryable=%t err=%v", errors.Is(err, ErrQuotaExhausted), Retryable(err), err)
			}
			if tc.quota && strings.Contains(err.Error(), "must not leak") {
				t.Fatal("quota error leaked response content")
			}
			if _, err := backend.call(context.Background(), chatRequest{}); err != nil {
				t.Fatalf("a later request must be able to recover after quota resets: %v", err)
			}
		})
	}
}
