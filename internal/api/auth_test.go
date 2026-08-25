package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerAuthRequiresExactCredential(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
		want   int
	}{
		{name: "missing", want: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic c2VjcmV0", want: http.StatusUnauthorized},
		{name: "empty bearer", header: "Bearer ", want: http.StatusUnauthorized},
		{name: "wrong token", header: "Bearer almost-secret", want: http.StatusUnauthorized},
		{name: "exact token", header: "Bearer secret", want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hit := false
			h := withRequestID(bearerAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hit = true
				w.WriteHeader(http.StatusNoContent)
			})))
			req := httptest.NewRequest(http.MethodGet, "/v1/plants", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			res := httptest.NewRecorder()

			h.ServeHTTP(res, req)

			if res.Code != tc.want {
				t.Fatalf("status = %d, want %d", res.Code, tc.want)
			}
			if hit != (tc.want == http.StatusNoContent) {
				t.Fatalf("handler hit = %v", hit)
			}
			if tc.want == http.StatusUnauthorized {
				var body map[string]string
				if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body["code"] != "unauthorized" || body["request_id"] == "" {
					t.Fatalf("body = %#v", body)
				}
			}
		})
	}
}
