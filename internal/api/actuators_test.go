package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/TheOutdoorProgrammer/planty/internal/api"
	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
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
	_, db, ctx := newServer(t)
	grown, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: "Actuator API test", Domain: plant.DomainHouseplant,
		Steward: plant.StewardSelf, Status: plant.StatusAlive, Location: "test",
		Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := &actuatorAPIHA{entities: []ha.Entity{
		{EntityID: "sensor.humidity", Domain: "sensor", FriendlyName: "Humidity", Available: true},
		{EntityID: "switch.grow", Domain: "switch", FriendlyName: "Grow light", State: "on", Available: true},
		{EntityID: "fan.cabinet", Domain: "fan", FriendlyName: "Cabinet fan", Available: true},
		{EntityID: "switch.plant_fan", Domain: "switch", FriendlyName: "Plant fan plug", Available: true},
	}}
	server := api.New(db, slog.New(slog.NewTextHandler(io.Discard, nil))).WithHomeAssistant(fake).Handler()

	discovered, body := do(t, server, http.MethodGet, "/v1/home-assistant/actuators", nil)
	if discovered.Code != http.StatusOK || body["count"] != float64(3) {
		t.Fatalf("discovery = %d %#v", discovered.Code, body)
	}
	rejected, _ := do(t, server, http.MethodPost, "/v1/actuators", map[string]any{"entity_id": "sensor.humidity", "name": "wrong"})
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("registered non-actuator with status %d", rejected.Code)
	}
	created, actuator := do(t, server, http.MethodPost, "/v1/actuators", map[string]any{
		"entity_id": "switch.plant_fan", "kind": "fan", "plant_ids": []string{grown.ID.String()},
	})
	if created.Code != http.StatusCreated || actuator["entity_id"] != "switch.plant_fan" || actuator["kind"] != "fan" {
		t.Fatalf("created = %d %#v", created.Code, actuator)
	}
	id := actuator["id"].(string)
	tooLong, _ := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/start", map[string]any{
		"duration_seconds": 3601, "actor": "Joey", "idempotency_key": uuid.NewString(),
	})
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("unbounded start status = %d", tooLong.Code)
	}
	started, lease := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/start", map[string]any{
		"duration_seconds": 60, "actor": "Dusk Planty plugin", "source": "agent", "idempotency_key": uuid.NewString(),
	})
	if started.Code != http.StatusCreated || lease["source"] != "agent" || len(fake.calls) != 1 || fake.calls[0] != "switch/turn_on:switch.plant_fan" {
		t.Fatalf("start = %d lease=%#v calls=%#v", started.Code, lease, fake.calls)
	}
	stopped, _ := do(t, server, http.MethodPost, "/v1/actuators/"+id+"/stop", map[string]any{
		"actor": "Dusk Planty plugin", "source": "agent", "idempotency_key": uuid.NewString(),
	})
	if stopped.Code != http.StatusOK || len(fake.calls) != 2 || fake.calls[1] != "switch/turn_off:switch.plant_fan" {
		t.Fatalf("stop = %d calls=%#v", stopped.Code, fake.calls)
	}
	fanScheduled, fanSchedule := do(t, server, http.MethodPut, "/v1/actuators/"+id+"/fan-schedule", map[string]any{
		"start_minute": 540, "end_minute": 1020, "timezone": "America/New_York",
		"enabled": true, "actor": "Joey",
	})
	if fanScheduled.Code != http.StatusOK || fanSchedule["start_minute"] != float64(540) {
		t.Fatalf("fan schedule = %d %#v", fanScheduled.Code, fanSchedule)
	}
	events, history := do(t, server, http.MethodGet, "/v1/actuators/"+id+"/events", nil)
	if events.Code != http.StatusOK || history["count"] != float64(5) {
		t.Fatalf("events = %d %#v", events.Code, history)
	}
	lightCreated, light := do(t, server, http.MethodPost, "/v1/actuators", map[string]any{
		"entity_id": "switch.grow", "kind": "light", "plant_ids": []string{grown.ID.String()},
	})
	if lightCreated.Code != http.StatusCreated || light["current_state"] != "on" {
		t.Fatalf("light registration = %d %#v", lightCreated.Code, light)
	}
	lightID := light["id"].(string)
	listed, listedBody := do(t, server, http.MethodGet, "/v1/actuators", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list actuators = %d %#v", listed.Code, listedBody)
	}
	var listedLight map[string]any
	for _, raw := range listedBody["actuators"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["id"] == lightID {
			listedLight = candidate
		}
	}
	if listedLight == nil || listedLight["current_state"] != "on" {
		t.Fatalf("listed light state = %#v, want on", listedLight)
	}
	scheduled, schedule := do(t, server, http.MethodPut, "/v1/actuators/"+lightID+"/light-schedule", map[string]any{
		"windows": []map[string]any{
			{"start_minute": 480, "end_minute": 540},
			{"start_minute": 720, "end_minute": 780},
		}, "timezone": "America/New_York",
		"enabled": true, "actor": "Joey",
	})
	windows, _ := schedule["windows"].([]any)
	if scheduled.Code != http.StatusOK || schedule["timezone"] != "America/New_York" ||
		schedule["start_minute"] != float64(480) || len(windows) != 2 {
		t.Fatalf("schedule = %d %#v", scheduled.Code, schedule)
	}
	storedSchedule, err := db.LightSchedule(ctx, uuid.MustParse(lightID))
	if err != nil || len(storedSchedule.Windows) != 2 {
		t.Fatalf("stored schedule = %#v err=%v", storedSchedule, err)
	}
	overlapping, _ := do(t, server, http.MethodPut, "/v1/actuators/"+lightID+"/light-schedule", map[string]any{
		"windows": []map[string]any{
			{"start_minute": 480, "end_minute": 600},
			{"start_minute": 540, "end_minute": 660},
		}, "timezone": "America/New_York", "enabled": true, "actor": "Joey",
	})
	if overlapping.Code != http.StatusBadRequest {
		t.Fatalf("overlapping schedule status = %d", overlapping.Code)
	}
	controlled, _ := do(t, server, http.MethodPost, "/v1/actuators/"+lightID+"/state", map[string]any{
		"on": true, "actor": "Joey",
	})
	if controlled.Code != http.StatusOK || fake.calls[len(fake.calls)-1] != "switch/turn_on:switch.grow" {
		t.Fatalf("light state = %d calls=%#v", controlled.Code, fake.calls)
	}
	updated, updatedActuator := do(t, server, http.MethodPatch, "/v1/actuators/"+lightID, map[string]any{
		"name": "Grow light", "kind": "water", "plant_ids": []string{grown.ID.String()},
	})
	if updated.Code != http.StatusOK || updatedActuator["kind"] != "water" {
		t.Fatalf("updated classification = %d %#v", updated.Code, updatedActuator)
	}
}
