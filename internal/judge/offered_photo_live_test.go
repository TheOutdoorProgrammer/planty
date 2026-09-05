package judge

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLiveKimiOpensOfferedPhoto(t *testing.T) {
	backend := liveOpenAI(t, "kimi-k3")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	out, err := backend.Judge(ctx, Request{
		System:    "Answer from the photograph. Do not guess its contents.",
		Turns:     []Turn{ask(text("Look at the latest image. What colour is it? One word."))},
		Offered:   []Offer{{Label: "today", Media: "image/png", Bytes: greenSquare(t)}},
		Schema:    probeSchema,
		Effort:    EffortMedium,
		MaxTokens: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	opened := false
	for _, step := range out.Steps {
		if step.Tool == "historical_photo" {
			if !strings.HasPrefix(step.Output, "Opened ") {
				t.Errorf("photo call failed: %+v", step)
			}
			opened = true
		}
	}
	if !opened {
		t.Error("the model did not open the photo")
	}
	if answer := decodeAnswer(t, out); !strings.Contains(strings.ToLower(answer), "green") {
		t.Errorf("the model did not see the photo: %q", answer)
	}
}
