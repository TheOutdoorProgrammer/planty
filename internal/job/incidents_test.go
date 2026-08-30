package job

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/pgtest"
	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

func TestIncidentRadarCorrelatesOnlyCompletedIndependentPlantSignals(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	makePlant := func(name string) plant.Plant {
		p, err := db.CreatePlant(ctx, plant.Plant{
			CommonName: name, Domain: plant.DomainHouseplant, Steward: plant.StewardSelf,
			Status: plant.StatusAlive, Location: "Incident office", Accessibility: plant.AccessEasy,
			WateringMethod: plant.WateringHand,
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first, second := makePlant("Radar first"), makePlant("Radar second")
	run, err := db.StartJudgmentRun(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []plant.Plant{first, second} {
		verdict, err := db.SaveVerdict(ctx, plant.Verdict{
			PlantID: p.ID, ForDate: time.Now().UTC(), Action: plant.ActionUrgent,
			Reasoning: "individual urgent action remains visible", Confidence: 0.8,
			Evidence: plant.Evidence{SensorSummary: "new anomaly"},
		})
		if err != nil || verdict.ID.String() == "" {
			t.Fatal(err)
		}
		if err := db.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{PlantID: p.ID, Succeeded: true, Attempts: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (IncidentRadar{Store: db}).Run(ctx, run.ID); err == nil {
		t.Fatal("unfinished run was correlated")
	}
	if err := db.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	incidents, err := (IncidentRadar{Store: db}).Run(ctx, run.ID)
	if err != nil || len(incidents) != 1 {
		t.Fatalf("incidents=%#v err=%v", incidents, err)
	}
	if incidents[0].SuspectedFactorType != plant.FactorLocation || !strings.Contains(incidents[0].Summary, "Dire shared condition") ||
		!strings.Contains(incidents[0].Reason, "Radar first was marked urgent. Agent reason: individual urgent action remains visible") ||
		len(incidents[0].Plants) != 2 {
		t.Fatalf("incident=%#v", incidents[0])
	}
}

func TestIncidentRadarIgnoresRoutineCheckActions(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, pgtest.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := db.StartJudgmentRun(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Routine first", "Routine second"} {
		p, err := db.CreatePlant(ctx, plant.Plant{
			CommonName: name, Domain: plant.DomainHouseplant, Steward: plant.StewardSelf,
			Status: plant.StatusAlive, Location: "Routine office", Accessibility: plant.AccessEasy,
			WateringMethod: plant.WateringHand,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.SaveVerdict(ctx, plant.Verdict{
			PlantID: p.ID, ForDate: time.Now().UTC(), Action: plant.ActionCheck,
			Reasoning: "routine follow-up", Confidence: 0.8,
			Evidence: plant.Evidence{SensorSummary: "ordinary care"},
		}); err != nil {
			t.Fatal(err)
		}
		if err := db.RecordJudgmentPlantResult(ctx, run.ID, store.JudgmentResultInput{PlantID: p.ID, Succeeded: true, Attempts: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.CompleteJudgmentRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	incidents, err := (IncidentRadar{Store: db}).Run(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 0 {
		t.Fatalf("routine check actions created incidents: %#v", incidents)
	}
}
