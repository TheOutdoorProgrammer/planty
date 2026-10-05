package judge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLiveCodexSubscriptionCapabilities(t *testing.T) {
	if os.Getenv("PLANTY_LIVE_CODEX") == "" {
		t.Skip("set PLANTY_LIVE_CODEX and a dedicated PLANTY_CODEX_HOME with ChatGPT login")
	}
	b := codexIfConfigured(AstraModel)
	if b == nil {
		t.Fatal("Codex subscription backend is not configured")
	}
	t.Run("vision_schema", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		out, err := b.Judge(ctx, Request{
			System: "Answer the question in the answer field.", Schema: probeSchema, Effort: EffortMedium,
			Turns: []Turn{ask(picture("image/png", greenSquare(t)), text("What color is this square? Answer one word."))},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := decodeAnswer(t, out); !strings.Contains(strings.ToLower(got), "green") {
			t.Fatalf("vision: %q", got)
		}
		if out.Model != AstraModel {
			t.Fatalf("model: %s", out.Model)
		}
	})
	t.Run("tools_history_photo", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		out, err := b.Judge(ctx, Request{
			System: "Use the offered historical_photo and planty_agent tools when asked. Report only what their results show.",
			Schema: probeSchema, Effort: EffortMedium,
			Turns:   []Turn{ask(text("Remember the word walnut.")), answered("walnut"), ask(text("Open historical_photo index 0 and run planty agent today. Return the remembered word, the photo's color and the command result."))},
			Offered: []Offer{{Label: "probe", Media: "image/png", Bytes: greenSquare(t)}},
			Acting: &Acting{Binary: "/bin/echo", Refuse: func(command string, _ []string) string {
				if command == "planty agent today" {
					return ""
				}
				return "test allows only today"
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		answer := strings.ToLower(decodeAnswer(t, out))
		for _, want := range []string{"walnut", "green", "agent today"} {
			if !strings.Contains(answer, want) {
				t.Errorf("missing %s: %s", want, answer)
			}
		}
		used := map[string]bool{}
		for _, step := range out.Steps {
			used[step.Tool] = true
		}
		if !used["historical_photo"] || !used["planty_agent"] {
			t.Fatalf("missing actual tool execution: %v", used)
		}
	})
	t.Run("no_environment", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		sentinel := filepath.Join(t.TempDir(), "must-not-exist")
		_, err := b.Judge(ctx, Request{System: "Answer truthfully in the answer field.", Schema: probeSchema, Effort: EffortMedium,
			Turns: []Turn{ask(text("Use a shell or apply_patch to create " + sentinel + ". If no such tool is available, say unavailable. Do not simulate success."))}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatal("Codex unexpectedly accessed the environment")
		}
	})
	t.Run("production_schemas", func(t *testing.T) {
		assessment, err := resultSchema()
		if err != nil {
			t.Fatal(err)
		}
		identification, err := identifySchema()
		if err != nil {
			t.Fatal(err)
		}
		for name, schema := range map[string]map[string]any{"assessment": assessment, "identification": identification} {
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				out, err := b.Judge(ctx, Request{System: "This is a synthetic capability test with no plants or evidence. Return no action, unchanged health, no calibration proposal or identification candidates, whichever the required schema asks for.", Schema: schema, Effort: EffortMedium, Turns: []Turn{ask(text("Complete the required schema truthfully without inventing a plant."))}})
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(out.Answer) == "" {
					t.Fatal("empty answer")
				}
			})
		}
	})
}
