package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/google/uuid"
)

type actuatorAPIHA struct {
	entities []ha.Entity
	calls    []string
}

func (h *actuatorAPIHA) Entities(context.Context) ([]ha.Entity, error) { return h.entities, nil }
func (h *actuatorAPIHA) CallService(_ context.Context, domain, service string, data map[string]any) error {
	h.calls = append(h.calls, domain+"/"+service+":"+data["entity_id"].(string))
	return nil
}

func TestActuatorAPIDiscoversRegistersAndControlsOnlyAllowlistedIDs(t *testing.T) {
	_, db, _ := newServer(t)
	fake := &actuatorAPIHA{entities: []ha.Entity{
		{EntityID: "sensor.humidity", Domain: "sensor", FriendlyName: "Humidity", Available: true},
		{EntityID: "light.grow", Domain: "light", FriendlyName: "Grow light", Available: true},
		{EntityID: "fan.cabinet", Domain: "fan", FriendlyName: "Cabinet fan", Available: true},
		{EntityID: "switch.plant_fan", Domain: "switch", FriendlyName: "Plant fan plug", Available: true},
	}}
	server := api.New(db, slog.New(slog.NewTextHandler(io.Discard, nil))).WithHomeAssistant(fake).Handler()

	discovered, body := do(t, server, http.MethodGet, "/v1/home-assistant/actuators", nil)
	if discovered.Code != http.StatusOK || body["count"] != float64(2) {
		t.Fatalf("discovery = %d %#v", discovered.Code, body)
	}
	rejected, _ := do(t, server, http.MethodPost, "/v1/actuators", map[string]any{"entity_id": "light.grow", "name": "wrong"})
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("registered non-actuator with status %d", rejected.Code)
	}
	created, actuator := do(t, server, http.MethodPost, "/v1/actuators", map[string]any{"entity_id": "switch.plant_fan"})
	if created.Code != http.StatusCreated || actuator["entity_id"] != "switch.plant_fan" {
		t.Fatalf("created = %d %#v", created.Code, actuator)
	}
	id := actuator["id"].(string)
	tooLong, _ := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/start", map[string]any{
		"duration_seconds": 3601, "actor": "Joey", "idempotency_key": uuid.NewString(),
	})
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("unbounded start status = %d", tooLong.Code)
	}
	started, _ := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/start", map[string]any{
		"duration_seconds": 60, "actor": "Joey", "idempotency_key": uuid.NewString(),
	})
	if started.Code != http.StatusCreated || len(fake.calls) != 1 || fake.calls[0] != "switch/turn_on:switch.plant_fan" {
		t.Fatalf("start = %d calls=%#v", started.Code, fake.calls)
	}
	stopped, _ := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/stop", map[string]any{
		"actor": "Joey", "idempotency_key": uuid.NewString(),
	})
	if stopped.Code != http.StatusOK || len(fake.calls) != 2 || fake.calls[1] != "switch/turn_off:switch.plant_fan" {
		t.Fatalf("stop = %d calls=%#v", stopped.Code, fake.calls)
	}
	events, history := do(t, server, http.MethodGet, "/v1/actuators/"+id+"/events", nil)
	if events.Code != http.StatusOK || history["count"] != float64(4) {
		t.Fatalf("events = %d %#v", events.Code, history)
	}
}
