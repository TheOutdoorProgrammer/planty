package plant

import (
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestHealthChangeRequiresOneShapeAndEvidence(t *testing.T) {
	plantID := uuid.New()
	baseline, delta := 75.0, -5.0
	valid := HealthChange{
		PlantID: plantID, Baseline: &baseline, Rationale: "visible growth",
		Evidence: HealthEvidence{Summary: "new leaf is fully opened"}, Source: SourceApp,
	}
	if err := valid.Valid(); err != nil {
		t.Fatalf("valid baseline: %v", err)
	}

	for name, change := range map[string]HealthChange{
		"neither shape": {PlantID: plantID, Rationale: "x", Evidence: HealthEvidence{Summary: "y"}, Source: SourceApp},
		"both shapes":   {PlantID: plantID, Baseline: &baseline, Delta: &delta, Rationale: "x", Evidence: HealthEvidence{Summary: "y"}, Source: SourceApp},
		"no evidence":   {PlantID: plantID, Delta: &delta, Rationale: "x", Source: SourceApp},
		"nan baseline":  {PlantID: plantID, Baseline: number(math.NaN()), Rationale: "x", Evidence: HealthEvidence{Summary: "y"}, Source: SourceApp},
	} {
		t.Run(name, func(t *testing.T) {
			if err := change.Valid(); err == nil {
				t.Fatal("invalid health change was accepted")
			}
		})
	}
}

func number(value float64) *float64 { return &value }
