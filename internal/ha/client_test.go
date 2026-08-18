package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// forecastServer answers the weather service call with the periods given.
func forecastServer(t *testing.T, entity string, periods []map[string]any) (*Client, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		w.Header().Set("Content-Type", "application/json")
		// The shape Home Assistant really returns: the entity map is nested
		// under service_response, alongside changed_states.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"changed_states": []any{},
			"service_response": map[string]any{
				entity: map[string]any{"forecast": periods},
			},
		})
	}))
	t.Cleanup(server.Close)

	return New(server.URL, "token"), &seen
}

func period(at time.Time, high, low float64) map[string]any {
	return map[string]any{
		"datetime":    at.Format(time.RFC3339),
		"temperature": high,
		"templow":     low,
		"condition":   "clear-night",
	}
}

func TestTonightLowTakesTheColdestPeriodInTheWindow(t *testing.T) {
	now := time.Now()
	client, _ := forecastServer(t, "weather.nws_home", []map[string]any{
		period(now.Add(2*time.Hour), 70, 54),
		period(now.Add(10*time.Hour), 68, 49),
		period(now.Add(40*time.Hour), 72, 38), // beyond the window
	})

	low, err := client.TonightLow(t.Context(), "weather.nws_home", 18*time.Hour)
	if err != nil {
		t.Fatalf("TonightLow: %v", err)
	}
	if low != 49 {
		t.Errorf("got %v, want 49: the coldest period inside the window", low)
	}
}

// A daily forecast stamps today's entry at the start of the day and hangs
// tonight's low on it, so by the afternoon the entry worth reading is already
// past-dated. Dropping past entries would throw away the one that matters.
func TestTonightLowKeepsTodaysEntryEvenOnceItIsPastDated(t *testing.T) {
	now := time.Now()
	client, _ := forecastServer(t, "weather.nws_home", []map[string]any{
		period(now.Add(-9*time.Hour), 74, 51), // today, stamped this morning
		period(now.Add(20*time.Hour), 76, 60), // tomorrow
	})

	low, err := client.TonightLow(t.Context(), "weather.nws_home", 18*time.Hour)
	if err != nil {
		t.Fatalf("TonightLow: %v", err)
	}
	if low != 51 {
		t.Errorf("got %v, want 51: today's entry carries tonight's low", low)
	}
}

// Anything older than a day describes a night that has been and gone, and
// acting on it would carry plants indoors for weather that already happened.
func TestTonightLowIgnoresAStaleForecast(t *testing.T) {
	now := time.Now()
	client, _ := forecastServer(t, "weather.nws_home", []map[string]any{
		period(now.Add(-30*time.Hour), 60, 28), // yesterday, far colder
		period(now.Add(-2*time.Hour), 70, 57),  // today
	})

	low, err := client.TonightLow(t.Context(), "weather.nws_home", 18*time.Hour)
	if err != nil {
		t.Fatalf("TonightLow: %v", err)
	}
	if low == 28 {
		t.Fatal("reported a low from a night that already passed")
	}
	if low != 57 {
		t.Errorf("got %v, want 57", low)
	}
}

func TestTonightLowSaysSoWhenTheWindowIsEmpty(t *testing.T) {
	now := time.Now()
	client, _ := forecastServer(t, "weather.nws_home", []map[string]any{
		period(now.Add(72*time.Hour), 70, 40),
	})

	if _, err := client.TonightLow(t.Context(), "weather.nws_home", 6*time.Hour); err == nil {
		t.Fatal("no usable forecast is an error, not a silent zero")
	}
}

// A zero would read as freezing and carry every plant indoors.
func TestTonightLowNeverReturnsASilentZero(t *testing.T) {
	client, _ := forecastServer(t, "weather.nws_home", nil)

	low, err := client.TonightLow(t.Context(), "weather.nws_home", 18*time.Hour)
	if err == nil {
		t.Fatalf("an empty forecast returned %v with no error", low)
	}
}

// Some integrations omit templow; the period's own temperature is the honest
// fallback rather than zero.
func TestLowFallsBackToTheTemperature(t *testing.T) {
	if got := (Forecast{Temperature: 61}).Low(); got != 61 {
		t.Errorf("got %v, want 61 when templow is absent", got)
	}

	templow := 44.0
	if got := (Forecast{Temperature: 61, TemplowRaw: &templow}).Low(); got != 44 {
		t.Errorf("got %v, want the reported low", got)
	}
}

func TestForecastNamesTheEntityItCouldNotFind(t *testing.T) {
	client, _ := forecastServer(t, "weather.somewhere_else", nil)

	_, err := client.Forecast(t.Context(), "weather.nws_home")
	if err == nil {
		t.Fatal("a forecast for the wrong entity should not pass silently")
	}
	if !strings.Contains(err.Error(), "weather.nws_home") {
		t.Errorf("the error should name what was asked for: %v", err)
	}
}

func TestStateParsesNumbersAndUnits(t *testing.T) {
	s := State{State: "42.5", Attributes: map[string]any{"unit_of_measurement": "%"}}

	got, err := s.Float()
	if err != nil {
		t.Fatalf("Float: %v", err)
	}
	if got != 42.5 {
		t.Errorf("got %v, want 42.5", got)
	}
	if s.Unit() != "%" {
		t.Errorf("got %q, want %%", s.Unit())
	}
}

// unavailable and unknown are normal for a battery sensor and must not parse
// into a number the judge would then reason about.
func TestUnavailableStateDoesNotParse(t *testing.T) {
	for _, raw := range []string{"unavailable", "unknown", ""} {
		if _, err := (State{State: raw}).Float(); err == nil {
			t.Errorf("%q parsed as a number", raw)
		}
	}
}

func TestErrorsCarryTheStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	_, err := New(server.URL, "wrong").State(t.Context(), "sensor.anything")
	if err == nil {
		t.Fatal("a 401 should be an error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("the error should say what happened: %v", err)
	}
}
