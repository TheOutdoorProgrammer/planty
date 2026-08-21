package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserWriteGuardRejectsCrossSiteWrite(t *testing.T) {
	t.Setenv("PLANTY_ALLOWED_BROWSER_HOSTS", "")
	hit := false
	h := browserWriteGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	req := httptest.NewRequest(http.MethodPost, "http://192.168.1.20/v1/notes", strings.NewReader(`{"body":"x"}`))
	req.Host = "192.168.1.20"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || hit {
		t.Fatalf("status=%d hit=%v, want 403 without handler", res.Code, hit)
	}
}

func TestBrowserWriteGuardAllowsSameOriginPrivateLAN(t *testing.T) {
	t.Setenv("PLANTY_ALLOWED_BROWSER_HOSTS", "")
	hit := false
	h := browserWriteGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://192.168.1.20/v1/notes", strings.NewReader(`{"body":"x"}`))
	req.Host = "192.168.1.20"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://192.168.1.20")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || !hit {
		t.Fatalf("status=%d hit=%v, want handler", res.Code, hit)
	}
}

func TestBrowserWriteGuardRejectsSimpleFormWrite(t *testing.T) {
	t.Setenv("PLANTY_ALLOWED_BROWSER_HOSTS", "")
	h := browserWriteGuard(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	}))
	req := httptest.NewRequest(http.MethodPost, "http://192.168.1.20/v1/notes", strings.NewReader("body=x"))
	req.Host = "192.168.1.20"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	if res.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d, want 415", res.Code)
	}
}

func TestBrowserWriteGuardLeavesNativeClientsAlone(t *testing.T) {
	t.Setenv("PLANTY_ALLOWED_BROWSER_HOSTS", "")
	hit := false
	h := browserWriteGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "http://planty.local/v1/verdicts/id/ack", nil)
	req.Host = "planty.local"
	res := httptest.NewRecorder()

	h.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent || !hit {
		t.Fatalf("status=%d hit=%v, want native request through", res.Code, hit)
	}
}
