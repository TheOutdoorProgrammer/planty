package plant

import (
	"testing"
	"time"
)

func TestEvidenceCoveragePrioritizesSafetyBeforeConvenience(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		item EvidenceCoverage
		want string
	}{
		{"identity", EvidenceCoverage{}, "Confirm the botanical identity"},
		{"toxicity", EvidenceCoverage{BotanicalKnown: true}, "Verify toxicity from a named source"},
		{"photo", EvidenceCoverage{BotanicalKnown: true, ToxicityChecked: true}, "Take a whole-plant baseline photo"},
		{"calibration", EvidenceCoverage{BotanicalKnown: true, ToxicityChecked: true, PhotoCount: 1, LatestPhoto: &now, HasSoilSensor: true}, "Calibrate the soil sensor"},
		{"health", EvidenceCoverage{BotanicalKnown: true, ToxicityChecked: true, PhotoCount: 1, LatestPhoto: &now}, "Establish health from current evidence"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			test.item.Prioritize(now)
			if test.item.NextBestInput != test.want {
				t.Fatalf("next input = %q, want %q", test.item.NextBestInput, test.want)
			}
		})
	}
}
