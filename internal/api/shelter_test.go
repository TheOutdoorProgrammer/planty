package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMalformedShelterJSONStopsBeforeTheStore(t *testing.T) {
	h := New(nil, slog.Default()).Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/shelter", strings.NewReader(`{"all":true`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body.String())
	}
}
