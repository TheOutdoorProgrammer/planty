package judge

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live proof that the CLI backend really answers, images and all. Opt in with
// PLANTY_LIVE_JUDGE=<path to a plant photo>.
func TestLiveCLIBackend(t *testing.T) {
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
	t.Logf("backend: %s", j.Backend())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	started := time.Now()
	candidates, err := j.Identify(ctx, Frame{Media: "image/jpeg", Bytes: raw}, Sighting{})
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	t.Logf("identify took %s", time.Since(started).Round(time.Millisecond))

	if len(candidates) == 0 {
		t.Fatal("no candidates came back")
	}
	for _, c := range candidates {
		t.Logf("  %s (%s) confidence %.2f", c.CommonName, c.ScientificName, c.Confidence)
	}
}
