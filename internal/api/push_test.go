package api_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestPushRegistrationReturnsServerAcceptanceAndHealthFindsIt(t *testing.T) {
	h, _, _ := newServer(t)
	installation := uuid.New()
	rec, body := do(t, h, http.MethodPost, "/v1/push-devices", map[string]any{
		"token": "aabbcc", "environment": "production", "installation_id": installation,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if body["accepted_at"] == nil || body["installation_id"] != installation.String() {
		t.Fatalf("registration did not return acceptance: %v", body)
	}

	rec, body = do(t, h, http.MethodGet,
		"/v1/push/health?environment=production&installation_id="+installation.String(), nil)
	if rec.Code != http.StatusOK || body["registration"] == nil {
		t.Fatalf("health did not recover registration: %d %v", rec.Code, body)
	}
}

func TestPushTestIsNotAnHTTPHealthCheck(t *testing.T) {
	h, _, _ := newServer(t)
	rec, _ := do(t, h, http.MethodPost, "/v1/push/test", map[string]any{
		"environment": "production", "installation_id": uuid.New(),
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("test without APNs credentials returned %d", rec.Code)
	}
}
