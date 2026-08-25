package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
)

func TestAgentHealthRemainsUnknownUntilBaselineAndWritesWithEvidence(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)
	unknown, err := runVerbCtx(t, ctx, deps, "health", "--plant", p.Slug)
	if err != nil || !strings.Contains(unknown, "health unknown") {
		t.Fatalf("unknown health = %q err=%v", unknown, err)
	}
	evidence, err := deps.Store.AddObservation(ctx, plant.Observation{
		PlantID: p.ID, Kind: plant.ObservedNote, Body: "upright leaves and active growth",
		OccurredAt: time.Now().UTC(), Source: plant.SourceAgent, Actor: "planty agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	key := uuid.NewString()
	written, err := runVerbCtx(t, ctx, deps, "healthchange", "--plant", p.Slug,
		"--baseline", "82", "--reason", "first inspection",
		"--evidence", "upright leaves and active growth", "--observation", evidence.ID.String(), "--key", key)
	if err != nil || !strings.Contains(written, "82.0%") {
		t.Fatalf("write = %q err=%v", written, err)
	}
	replayed, err := runVerbCtx(t, ctx, deps, "healthchange", "--plant", p.Slug,
		"--baseline", "82", "--reason", "first inspection",
		"--evidence", "upright leaves and active growth", "--observation", evidence.ID.String(), "--key", key)
	if err != nil || !strings.Contains(replayed, "already recorded") {
		t.Fatalf("replay = %q err=%v", replayed, err)
	}
	listed, err := runVerbCtx(t, ctx, deps, "health", "--plant", p.Slug)
	if err != nil || !strings.Contains(listed, "82% health") || !strings.Contains(listed, "planty agent") {
		t.Fatalf("listed = %q err=%v", listed, err)
	}
}

func TestAgentHealthChangeRejectsUnresolvableEvidence(t *testing.T) {
	deps, p, ctx := toxicityDeps(t)
	_, err := runVerbCtx(t, ctx, deps, "healthchange", "--plant", p.Slug,
		"--baseline", "82", "--reason", "first inspection",
		"--evidence", "looks healthy", "--key", uuid.NewString())
	if err == nil || !strings.Contains(err.Error(), "--photo, --observation, or --reading") {
		t.Fatalf("free-text-only evidence returned %v", err)
	}

	_, err = runVerbCtx(t, ctx, deps, "healthchange", "--plant", p.Slug,
		"--baseline", "82", "--reason", "first inspection",
		"--evidence", "looks healthy", "--photo", uuid.NewString(), "--key", uuid.NewString())
	if err == nil || !strings.Contains(err.Error(), "not belonging to this plant") {
		t.Fatalf("unknown photo evidence returned %v", err)
	}
}
