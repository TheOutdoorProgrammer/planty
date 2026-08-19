package job

import (
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

// notification is one thing the house was told, so a test can assert what the
// operator would actually have seen rather than only that a call happened.
type notification struct {
	service string
	title   string
	message string
}

// fakeHA stands in for Home Assistant: it answers the forecast and records
// every notification and announcement.
type fakeHA struct {
	server *httptest.Server

	entity  string
	periods []map[string]any

	notified  []notification
	announced []string
	services  []string
}

func newFakeHA(t *testing.T, entity string) *fakeHA {
	t.Helper()
	f := &fakeHA{entity: entity}

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasPrefix(r.URL.Path, "/api/services/weather/get_forecasts"):
			// The shape Home Assistant really returns. A fake that flattened
			// this agreed with the bug and the cold watch never once ran.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"changed_states": []any{},
				"service_response": map[string]any{
					f.entity: map[string]any{"forecast": f.periods},
				},
			})

		case strings.HasPrefix(r.URL.Path, "/api/services/notify/"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			title, _ := body["title"].(string)
			message, _ := body["message"].(string)
			f.notified = append(f.notified, notification{
				service: strings.TrimPrefix(r.URL.Path, "/api/services/notify/"),
				title:   title,
				message: message,
			})
			_, _ = w.Write([]byte(`[]`))

		case strings.HasPrefix(r.URL.Path, "/api/services/script/announce"):
			var body struct {
				Variables struct {
					Message string `json:"message"`
				} `json:"variables"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.announced = append(f.announced, body.Variables.Message)
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

// forecast sets a single daily period stamped this morning, which is the shape
// Home Assistant actually returns: today's entry carries tonight's low.
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
