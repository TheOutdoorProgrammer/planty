package judge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Live proof that offered photographs are genuinely optional: the same record
// with the same photograph, asked two questions, one of which needs to look.
func TestLiveConsultOffersRatherThanForces(t *testing.T) {
	photo := os.Getenv("PLANTY_LIVE_JUDGE")
	if photo == "" {
		t.Skip("set PLANTY_LIVE_JUDGE to a plant photo to run this")
	}
	raw, err := os.ReadFile(photo)
	if err != nil {
		t.Fatalf("read photo: %v", err)
	}

	t.Setenv("PLANTY_JUDGE", "cli")
	t.Setenv("ANTHROPIC_API_KEY", "")
	j := New()
	if j == nil {
		t.Fatal("no backend; is the claude CLI installed and signed in?")
	}

	watered := time.Now().Add(-3 * 24 * time.Hour)
	history := History{
		Plant: plant.Plant{
			CommonName: "Golden pothos", BotanicalName: "Epipremnum aureum",
			Location: "Living room", WateringMethod: plant.WateringHand,
		},
		Observations: []plant.Observation{
			{Kind: plant.ObservedWatered, OccurredAt: watered, Source: plant.SourceApp},
		},
		Readings: []Sample{{TakenAt: time.Now().Add(-time.Hour), Fraction: 0.71}},
	}
	offered := []Offer{{Label: "18 August, 2 hours ago", Media: "image/jpeg", Bytes: raw}}

	for _, tc := range []struct{ name, asked string }{
		{"record answers it", "when did I last water this and does it need water today?"},
		{"a picture answers it", "what colour are the leaves in the most recent photo?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			started := time.Now()
			answer, err := j.Consult(ctx, history, offered, tc.asked, nil)
			if err != nil {
				t.Fatalf("consult: %v", err)
			}
			t.Logf("took %s, looked_at=%q", time.Since(started).Round(time.Millisecond), answer.LookedAt)
			t.Logf("reply: %s", answer.Reply)

			if answer.Reply == "" {
				t.Error("no reply came back")
			}
		})
	}
}
