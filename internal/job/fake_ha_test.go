package job

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
)

type notification struct {
	title   string
	message string
}

// fakeHA stands in for Home Assistant's data and actuator APIs while also
// implementing Notifier as a separate direct channel for job tests. If a job
// tries to call HA's old notify/script services, haNotifications records it.
type fakeHA struct {
	server *httptest.Server

	entity  string
	periods []map[string]any

	notified        []notification
	haNotifications []string
	services        []string
}

func newFakeHA(t *testing.T, entity string) *fakeHA {
	t.Helper()
	f := &fakeHA{entity: entity}

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/services/weather/get_forecasts"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"changed_states": []any{},
				"service_response": map[string]any{
					f.entity: map[string]any{"forecast": f.periods},
				},
			})

		case strings.HasPrefix(r.URL.Path, "/api/services/notify/"),
			strings.HasPrefix(r.URL.Path, "/api/services/script/announce"):
			f.haNotifications = append(f.haNotifications, r.URL.Path)
			_, _ = w.Write([]byte(`[]`))

		case strings.HasPrefix(r.URL.Path, "/api/services/switch/"):
			f.services = append(f.services, r.URL.Path)
			_, _ = w.Write([]byte(`[]`))

		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeHA) client() *ha.Client { return ha.New(f.server.URL, "token") }

func (f *fakeHA) Send(_ context.Context, title, body string, _ map[string]any) error {
	f.notified = append(f.notified, notification{title: title, message: body})
	return nil
}

func (f *fakeHA) forecast(high, low float64) {
	f.periods = []map[string]any{{
		"datetime":    time.Now().Add(-6 * time.Hour).Format(time.RFC3339),
		"temperature": high,
		"templow":     low,
	}}
}

func (f *fakeHA) said(fragment string) bool {
	for _, n := range f.notified {
		if strings.Contains(n.title+" "+n.message, fragment) {
			return true
		}
	}
	return false
}

func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
