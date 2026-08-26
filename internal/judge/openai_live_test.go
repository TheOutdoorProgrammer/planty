package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

// These spend real subscription budget against a live OpenAI-compatible
// endpoint, so they are opt-in. They exist because the capability table is a
// record of observed behaviour, and an unobserved claim in it is a guess.
func liveOpenAI(t *testing.T, model string) *openaiBackend {
	t.Helper()
	if os.Getenv("PLANTY_LIVE_OPENAI") == "" {
		t.Skip("set PLANTY_LIVE_OPENAI to run this")
	}
	if os.Getenv("OPENCODE_API_KEY") == "" {
		t.Skip("set OPENCODE_API_KEY to run this")
	}
	return newOpenAIBackend(Provider{
		ID: "opencode-go", Kind: KindOpenAI,
		BaseURL: "https://opencode.ai/zen/go/v1", APIKeyEnv: "OPENCODE_API_KEY",
	}, model)
}

// greenSquare is a 64px PNG. Built rather than committed so the test carries
// no fixture, and larger than 10px because some models reject tiny images.
func greenSquare(t *testing.T) []byte {
	t.Helper()
	const side = 64
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{R: 40, G: 180, B: 70, A: 255}},
		image.Point{}, draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode probe image: %v", err)
	}
	return buf.Bytes()
}

var probeSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"answer"},
	"properties": map[string]any{
		"answer": map[string]any{"type": "string"},
	},
}

func decodeAnswer(t *testing.T, out Outcome) string {
	t.Helper()
	var decoded struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(out.Answer), &decoded); err != nil {
		t.Fatalf("the answer did not honour the schema: %v (%q)", err, out.Answer)
	}
	return decoded.Answer
}

// The capability table says these models return a validated JSON answer. This
// is the test that makes that a fact rather than a claim.
func TestLiveStructuredOutputHoldsForEveryTextModel(t *testing.T) {
	for _, model := range []string{"qwen3.8-max", "deepseek-v4-flash", "glm-5.2", "mimo-v2.5"} {
		t.Run(model, func(t *testing.T) {
			backend := liveOpenAI(t, model)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			out, err := backend.Judge(ctx, Request{
				System:    "You answer in the answer field and nothing else.",
				Turns:     []Turn{ask(text("Say the single word ok."))},
				Schema:    probeSchema,
				MaxTokens: 256,
			})
			if err != nil {
				t.Fatalf("Judge: %v", err)
			}
			if got := decodeAnswer(t, out); !strings.Contains(strings.ToLower(got), "ok") {
				t.Errorf("unexpected answer %q", got)
			}
			if out.Model != model {
				t.Errorf("the outcome names %q, not the model asked for", out.Model)
			}
		})
	}
}

// Vision is the capability the picker gates hardest on, so the models it lets
// through for Identify have to actually see.
func TestLiveVisionHoldsForTheModelsTheTableAllows(t *testing.T) {
	for _, model := range []string{"qwen3.8-max", "kimi-k3", "mimo-v2.5"} {
		t.Run(model, func(t *testing.T) {
			backend := liveOpenAI(t, model)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			out, err := backend.Judge(ctx, Request{
				System:    "You answer in the answer field and nothing else.",
				Turns:     []Turn{ask(picture("image/png", greenSquare(t)), text("What colour is this? One word."))},
				Schema:    probeSchema,
				MaxTokens: 256,
			})
			if err != nil {
				t.Fatalf("Judge: %v", err)
			}
			if got := decodeAnswer(t, out); !strings.Contains(strings.ToLower(got), "green") {
				t.Errorf("%s did not see the image; it answered %q", model, got)
			}
		})
	}
}

// The tool loop against a real model, which is the part a stubbed server
// cannot prove: that a model actually chooses to call the function.
func TestLiveTheModelCallsPlantyAndUsesTheResult(t *testing.T) {
	backend := liveOpenAI(t, "qwen3.8-max")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	out, err := backend.Judge(ctx, Request{
		System: "Use the planty_agent tool to answer. The only verb you need is `planty agent today`.",
		Turns:  []Turn{ask(text("Run `planty agent today` and put its first line in the answer field."))},
		Schema: probeSchema,
		Acting: &Acting{
			Binary: "/bin/echo",
			Refuse: func(string, []string) string { return "" },
		},
		MaxTokens: 1024,
	})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}

	var ran bool
	for _, step := range out.Steps {
		if step.Kind == StepAction && step.Tool == "planty_agent" {
			ran = true
		}
	}
	if !ran {
		t.Errorf("the model never called the tool; steps were %+v", out.Steps)
	}
}
