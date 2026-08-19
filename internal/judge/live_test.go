package judge

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// These spend real tokens against whatever backend is configured, so they are
// opt-in. They exist because every bug this package has actually shipped was
// invisible to a unit test: a tool list that swallowed the prompt, a narrator
// that called a living plant dead, a cancellation flattened into a string.
func liveJudge(t *testing.T, envVar string) *Judge {
	t.Helper()
	if os.Getenv(envVar) == "" {
		t.Skip("set " + envVar + " to run this")
	}

	t.Setenv("PLANTY_JUDGE", "cli")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if os.Getenv("PLANTY_JUDGE_MODEL") == "" {
		t.Setenv("PLANTY_JUDGE_MODEL", "claude-haiku-4-5-20251001")
	}

	j := New()
	if j == nil {
		t.Fatal("no backend; is the claude CLI installed and signed in?")
	}
	return j
}

func liveContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// / The daily verdict is text-only, and the text-only path is the one that ends
// in a tool list. A prompt following that list is read as one more tool name.
func TestLiveTextOnlyJudgmentReachesTheModel(t *testing.T) {
	j := liveJudge(t, "PLANTY_LIVE_ASSESS")
	fraction := 0.82

	result, err := j.Assess(liveContext(t), Evidence{
		Plant: plant.Plant{CommonName: "Golden pothos", WateringMethod: plant.WateringHand},
		Sensors: []SensorState{{
			Role: plant.RoleSoilMoisture, Fraction: &fraction,
			Calibrated: true, TakenAt: time.Now().Add(-time.Hour),
		}},
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if result.Action == "" {
		t.Error("a verdict with no action")
	}
	t.Logf("action=%s confidence=%.2f", result.Action, result.Confidence)
}

// Set PLANTY_LIVE_JUDGE to a plant photo.
func TestLiveIdentifyReadsARealPhotograph(t *testing.T) {
	j := liveJudge(t, "PLANTY_LIVE_JUDGE")
	raw, err := os.ReadFile(os.Getenv("PLANTY_LIVE_JUDGE"))
	if err != nil {
		t.Fatalf("read photo: %v", err)
	}

	candidates, err := j.Identify(liveContext(t), Frame{Media: "image/jpeg", Bytes: raw}, Sighting{})
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("no candidates came back")
	}
	for _, c := range candidates {
		t.Logf("  %s (%s) %.2f", c.CommonName, c.ScientificName, c.Confidence)
	}
}

// Photographs are offered, not attached: a question the record answers should
// cost nothing extra, and the reply says which it opened.
func TestLiveOfferedPhotographsAreOptional(t *testing.T) {
	j := liveJudge(t, "PLANTY_LIVE_JUDGE")
	raw, err := os.ReadFile(os.Getenv("PLANTY_LIVE_JUDGE"))
	if err != nil {
		t.Fatalf("read photo: %v", err)
	}

	watered := time.Now().Add(-3 * 24 * time.Hour)
	history := History{
		Plant: plant.Plant{CommonName: "Golden pothos", Location: "Living room"},
		Observations: []plant.Observation{
			{Kind: plant.ObservedWatered, OccurredAt: watered, Source: plant.SourceApp},
		},
		Readings: []Sample{{TakenAt: time.Now().Add(-time.Hour), Fraction: 0.71}},
	}
	offered := []Offer{{Label: "today", Media: "image/jpeg", Bytes: raw}}

	for _, tc := range []struct{ name, asked string }{
		{"the record answers it", "when did I last water this?"},
		{"only a picture answers it", "what colour are the leaves right now?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer, err := j.Consult(liveContext(t), history, offered, tc.asked, nil, uuid.New())
			if err != nil {
				t.Fatalf("consult: %v", err)
			}
			t.Logf("looked_at=%q", answer.LookedAt)
			t.Logf("reply: %s", answer.Reply)
		})
	}
}

// A follow-up must continue the session rather than re-read the record, which
// is the difference between a flat cost per turn and one that climbs.
func TestLiveFollowUpsResumeRatherThanReread(t *testing.T) {
	j := liveJudge(t, "PLANTY_LIVE_SESSION")

	observations := make([]plant.Observation, 0, 40)
	for i := 40; i > 0; i-- {
		observations = append(observations, plant.Observation{
			Kind:       plant.ObservedWatered,
			OccurredAt: time.Now().AddDate(0, 0, -i),
			Source:     plant.SourceApp,
			Body:       "routine watering, soil dry two inches down, drained freely",
		})
	}
	history := History{
		Plant:        plant.Plant{CommonName: "Golden pothos", Location: "Living room"},
		Observations: observations,
	}

	conversation := uuid.New()
	var prior []PriorAnswer

	for i, question := range []string{
		"how often have I been watering this?",
		"is that too often?",
		"what should I change?",
	} {
		started := time.Now()
		answer, err := j.Consult(liveContext(t), history, nil, question, prior, conversation)
		if err != nil {
			t.Fatalf("turn %d: %v", i+1, err)
		}
		t.Logf("turn %d (%s): %s", i+1, time.Since(started).Round(time.Millisecond),
			answer.Reply[:min(90, len(answer.Reply))])
		prior = append(prior, PriorAnswer{Asked: question, Reply: answer})
	}
}
