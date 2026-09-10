package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/ha"
	"github.com/TheOutdoorProgrammer/planty/internal/job"
	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
	"github.com/google/uuid"
)

func TestActuatorJobRestoresSchedulesAfterConnectionRefusal(t *testing.T) {
	ctx := t.Context()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	subject, err := db.CreatePlant(ctx, plant.Plant{
		CommonName: "Recovery test", Domain: plant.DomainHouseplant, Steward: plant.StewardSelf,
		Status: plant.StatusAlive, Location: "test", Accessibility: plant.AccessEasy, WateringMethod: plant.WateringHand,
	})
	if err != nil {
		t.Fatal(err)
	}
	var actuators []plant.Actuator
	for _, kind := range []plant.ActuatorKind{plant.ActuatorLight, plant.ActuatorFan} {
		actuator, err := db.RegisterActuator(ctx, plant.Actuator{
			EntityID: "switch.recovery_" + string(kind), Name: "Recovery " + string(kind), Kind: kind, PlantIDs: []uuid.UUID{subject.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		schedule := plant.ActuatorSchedule{ActuatorID: actuator.ID, StartMinute: 8 * 60, EndMinute: 20 * 60, Timezone: "UTC", Enabled: false}
		if kind == plant.ActuatorLight {
			_, err = db.SetLightSchedule(ctx, schedule, "test", plant.SourceApp)
		} else {
			_, err = db.SetFanSchedule(ctx, schedule, "test", plant.SourceApp)
		}
		if err != nil {
			t.Fatal(err)
		}
		actuators = append(actuators, actuator)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	control := job.ActuatorControl{Store: db, HA: ha.New("http://"+address, "test"), Log: log}
	var mu sync.Mutex
	states := map[string]string{}
	for _, actuator := range actuators {
		states[actuator.EntityID] = "on"
	}
	requests := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/states/") {
			entity := strings.TrimPrefix(r.URL.Path, "/api/states/")
			state := states[entity]
			if state == "on" {
				state = "unknown"
				if strings.HasSuffix(entity, "fan") {
					state = "unavailable"
				}
			}
			_ = json.NewEncoder(w).Encode(ha.State{EntityID: entity, State: state})
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/services/switch/turn_off" {
			t.Errorf("unexpected actuator request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var data struct {
			EntityID string `json:"entity_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, ok := states[data.EntityID]; !ok {
			t.Errorf("unregistered target: %s", data.EntityID)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests++
		states[data.EntityID] = "off"
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	attempts := 0
	recovering := actuatorReconcileFunc(func(ctx context.Context, now time.Time) (int, error) {
		attempts++
		changed, reconcileErr := control.Reconcile(ctx, now)
		if attempts == 1 {
			if changed != 0 || !errors.Is(reconcileErr, syscall.ECONNREFUSED) {
				t.Fatalf("original failure not reproduced: changed=%d err=%v", changed, reconcileErr)
			}
			_ = server.Listener.Close()
			server.Listener, err = net.Listen("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			server.Start()
		}
		return changed, reconcileErr
	})
	if err := runActuatorReconciliation(ctx, recovering, log); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 || requests != 2 {
		t.Fatalf("attempts=%d applied switch-off requests=%d", attempts, requests)
	}
	for _, actuator := range actuators {
		if states[actuator.EntityID] != "off" {
			t.Fatalf("actuator %s was not switched off", actuator.Kind)
		}
		events, err := db.ActuatorEvents(ctx, actuator.ID, 20)
		if err != nil {
			t.Fatal(err)
		}
		failed, applied := 0, 0
		for _, event := range events {
			switch event.Action {
			case "schedule_failed":
				failed++
			case "state_changed":
				applied++
			}
		}
		if failed != 1 || applied != 1 {
			t.Fatalf("%s audit lost failure or recovery: failed=%d applied=%d", actuator.Kind, failed, applied)
		}
	}
}
