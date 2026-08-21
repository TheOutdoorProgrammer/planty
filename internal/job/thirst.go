package job

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

// Thirst says which plants their own probes call dry, and moves no water.
type Thirst struct {
	Store         *store.Store
	Log           *slog.Logger
	Notifications Notifier
}

func (t Thirst) Run(ctx context.Context) error {
	plants, err := t.Store.ListPlants(ctx, store.PlantFilter{Status: plant.StatusAlive})
	if err != nil {
		return fmt.Errorf("list plants: %w", err)
	}

	var dry []plant.Plant
	var heard int
	for _, p := range plants {
		fraction, spoke := moisture(ctx, t.Store, p)
		if !spoke {
			continue
		}
		heard++
		if fraction <= Thirsty {
			dry = append(dry, p)
		}
	}

	if heard == 0 {
		t.Log.Info("no calibrated probes, so nothing can be said about thirst",
			"plants", len(plants))
		return nil
	}
	if len(dry) == 0 {
		t.Log.Info("nothing is dry", "probed", heard)
		return nil
	}

	sort.SliceStable(dry, func(i, j int) bool { return dry[i].Risk() > dry[j].Risk() })

	t.Log.Info("plants are dry", "count", len(dry), "probed", heard)
	return notify(ctx, t.Notifications, "Plants want water", thirstMessage(dry), map[string]any{
		"data": map[string]any{"tag": "planty-thirst"},
	})
}

func thirstMessage(dry []plant.Plant) string {
	var b strings.Builder
	b.WriteString("Their own probes say these are dry:\n")

	for _, p := range dry {
		fmt.Fprintf(&b, "\n- %s", p.CommonName)
		if p.Location != "" {
			fmt.Fprintf(&b, " (%s)", p.Location)
		}
		if p.IsFriends() {
			fmt.Fprintf(&b, " - %s's", p.Steward)
		}
		if p.WateringMethod == plant.WateringLetPot {
			b.WriteString(" - on the LetPot line")
		}
	}

	b.WriteString("\n\nPlanty does not water anything. Run the pump or fill a can yourself.")
	return b.String()
}
