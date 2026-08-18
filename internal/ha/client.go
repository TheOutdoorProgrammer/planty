// Package ha talks to Home Assistant over its REST API: sensor states, weather
// forecasts, notifications and service calls.
package ha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is a Home Assistant REST client.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New builds a client against a base URL such as https://home.example.com.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 20 * time.Second},
	}
}

// State is one entity's current state.
type State struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"last_changed"`
}

// Float parses the state as a number, which most sensors report.
func (s State) Float() (float64, error) { return strconv.ParseFloat(s.State, 64) }

// Unit returns the reported unit of measurement, if any.
func (s State) Unit() string {
	if u, ok := s.Attributes["unit_of_measurement"].(string); ok {
		return u
	}
	return ""
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("home assistant %s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// State fetches one entity.
func (c *Client) State(ctx context.Context, entityID string) (State, error) {
	var s State
	err := c.do(ctx, http.MethodGet, "/api/states/"+url.PathEscape(entityID), nil, &s)
	return s, err
}

// States fetches every entity.
func (c *Client) States(ctx context.Context) ([]State, error) {
	var out []State
	err := c.do(ctx, http.MethodGet, "/api/states", nil, &out)
	return out, err
}

// CallService invokes a service, for example notify/notify.
func (c *Client) CallService(ctx context.Context, domain, service string, data map[string]any) error {
	return c.do(ctx, http.MethodPost, "/api/services/"+domain+"/"+service, data, nil)
}

// Notify sends a push notification through a notify service.
func (c *Client) Notify(ctx context.Context, service, title, message string, extra map[string]any) error {
	data := map[string]any{"title": title, "message": message}
	maps.Copy(data, extra)
	return c.CallService(ctx, "notify", service, data)
}

// Announce speaks through the house speakers via the existing script.
func (c *Client) Announce(ctx context.Context, message string) error {
	return c.CallService(ctx, "script", "announce", map[string]any{
		"variables": map[string]any{"message": message},
	})
}

// Forecast is one predicted period.
type Forecast struct {
	DateTime    time.Time `json:"datetime"`
	Temperature float64   `json:"temperature"`
	TemplowRaw  *float64  `json:"templow"`
	Condition   string    `json:"condition"`
}

// Low returns the period's overnight low, falling back to its temperature.
func (f Forecast) Low() float64 {
	if f.TemplowRaw != nil {
		return *f.TemplowRaw
	}
	return f.Temperature
}

// Forecast reads a weather entity's daily forecast.
func (c *Client) Forecast(ctx context.Context, entityID string) ([]Forecast, error) {
	body := map[string]any{
		"entity_id": entityID,
		"type":      "daily",
	}
	// The entity map is nested under service_response, alongside changed_states.
	// Decoding the top level as the map reads changed_states, which is an array.
	var resp struct {
		ServiceResponse map[string]struct {
			Forecast []Forecast `json:"forecast"`
		} `json:"service_response"`
	}
	err := c.do(ctx, http.MethodPost,
		"/api/services/weather/get_forecasts?return_response=true", body, &resp)
	if err != nil {
		return nil, err
	}
	if entry, ok := resp.ServiceResponse[entityID]; ok {
		return entry.Forecast, nil
	}
	return nil, fmt.Errorf("no forecast returned for %s", entityID)
}

// StaleForecast bounds how far back a period may be dated. Today's daily entry
// is past-dated by the afternoon yet carries tonight's low, so it must survive.
const StaleForecast = 24 * time.Hour

// TonightLow returns the lowest temperature forecast within the window.
func (c *Client) TonightLow(ctx context.Context, entityID string, within time.Duration) (float64, error) {
	periods, err := c.Forecast(ctx, entityID)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	cutoff, earliest := now.Add(within), now.Add(-StaleForecast)

	low := 0.0
	found := false
	for _, p := range periods {
		if p.DateTime.After(cutoff) || p.DateTime.Before(earliest) {
			continue
		}
		if !found || p.Low() < low {
			low, found = p.Low(), true
		}
	}
	if !found {
		return 0, fmt.Errorf("no usable forecast for %s within %s", entityID, within)
	}
	return low, nil
}
