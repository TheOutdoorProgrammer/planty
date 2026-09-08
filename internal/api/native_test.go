package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeRoutesKeepSeparateTrustBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, token, configured string
		want                                  int
	}{
		{"missing app credential", "POST", "/v1/native-telemetry", "", "app-secret", 401},
		{"wrong app credential", "POST", "/v1/native-telemetry", "wrong", "app-secret", 401},
		{"authenticated relay", "POST", "/v1/native-telemetry", "app-secret", "app-secret", 204},
		{"unconfigured auth disables relay", "POST", "/v1/native-telemetry", "", "", 503},
		{"publisher uses own identity", "PUT", "/v1/native-symbols/image/arm64", "oidc-publisher", "app-secret", 202},
		{"app cannot publish", "PUT", "/v1/native-symbols/image/arm64", "app-secret", "app-secret", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := New(nil, nil).WithBearerToken(tc.configured).
				WithNativeTelemetry(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).
				WithNativeSymbols(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer oidc-publisher" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					if r.PathValue("image_uuid") != "image" || r.PathValue("architecture") != "arm64" {
						t.Error("missing symbol route parameters")
					}
					w.WriteHeader(http.StatusAccepted)
				}))
			request := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status=%d, want=%d", response.Code, tc.want)
			}
		})
	}
}
