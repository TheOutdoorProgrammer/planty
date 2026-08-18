package judge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// MaxTimelineImages caps one diagnosis. Enough to show a trend, few enough that
// the oldest frame is still the same plant in the same pot.
const MaxTimelineImages = 6

// Frame is one dated image of a plant.
type Frame struct {
	TakenAt time.Time
	Media   string
	Bytes   []byte
	Caption string
}

// Diagnosis is what the model saw across a plant's photo timeline.
type Diagnosis struct {
	Finding    string   `json:"finding"`
	Cause      string   `json:"likely_cause"`
	Action     string   `json:"suggested_action"`
	Changed    bool     `json:"changed_over_time"`
	Confidence float64  `json:"confidence"`
	Watch      []string `json:"watch_for"`
}

const visionSystem = `You are looking at dated photographs of one houseplant, oldest first.

Your job is the thing sensors cannot do: say what has visibly CHANGED between
the earliest and latest frame, and what that change means.

- Compare frames. A single sad-looking photo is far less informative than the
  same plant getting worse over three weeks, and the comparison is why these
  images were sent together.
- Yellowing lower leaves that spread upward over time reads differently from
  yellowing that appeared all at once.
- Say plainly when the photos show nothing wrong. Most plants are fine, and
  inventing a problem is worse than missing a subtle one.
- Say plainly when the photos are too few, too dark, or too similar to judge.
  Low confidence is a real answer.
- Distinguish overwatering from underwatering explicitly when you can. They look
  similar to a beginner and the corrective actions are opposite, so guessing
  wrong actively causes harm.

One clear finding a beginner can act on. No lists of possibilities.`

// Diagnose reads a plant's photo timeline and reports what changed.
func (j *Judge) Diagnose(ctx context.Context, p plant.Plant, frames []Frame) (Diagnosis, error) {
	if len(frames) == 0 {
		return Diagnosis{}, fmt.Errorf("no photographs to look at")
	}
	if len(frames) > MaxTimelineImages {
		// Keep the most recent, since the newest frame is the one being asked about.
		frames = frames[len(frames)-MaxTimelineImages:]
	}

	schema, err := diagnosisSchema()
	if err != nil {
		return Diagnosis{}, err
	}

	blocks := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock(visionPreamble(p, frames)),
	}
	for _, f := range frames {
		blocks = append(blocks,
			anthropic.NewTextBlock(fmt.Sprintf("Taken %s ago%s:", ago(f.TakenAt), caption(f))),
			anthropic.NewImageBlockBase64(f.Media, base64.StdEncoding.EncodeToString(f.Bytes)),
		)
	}

	message, err := j.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(j.model),
		MaxTokens: 2048,
		System:    []anthropic.TextBlockParam{{Text: visionSystem}},
		Messages:  []anthropic.MessageParam{{Role: anthropic.MessageParamRoleUser, Content: blocks}},
		OutputConfig: anthropic.OutputConfigParam{
			// Comparing frames over time is the hard part; worth the depth.
			Effort: anthropic.OutputConfigEffortHigh,
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	})
	if err != nil {
		return Diagnosis{}, err
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return Diagnosis{}, ErrRefused
	}

	for _, block := range message.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			var out Diagnosis
			if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
				return Diagnosis{}, fmt.Errorf("decode diagnosis: %w", err)
			}
			return out, nil
		}
	}
	return Diagnosis{}, fmt.Errorf("no diagnosis in response")
}

func caption(f Frame) string {
	if f.Caption == "" {
		return ""
	}
	return " (" + f.Caption + ")"
}

func visionPreamble(p plant.Plant, frames []Frame) string {
	span := "a single moment"
	if len(frames) > 1 {
		span = fmt.Sprintf("%s, oldest first", ago(frames[0].TakenAt))
	}
	return fmt.Sprintf(
		"%s (%s), kept in %s with %s light, watered by %s. %d photographs spanning %s.",
		p.CommonName, orUnknown(p.BotanicalName), orUnknown(p.Location),
		orUnknown(string(p.LightExposure)), p.WateringMethod, len(frames), span)
}

func orUnknown(s string) string {
	if s == "" {
		return "unrecorded"
	}
	return s
}

func diagnosisSchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["finding", "likely_cause", "suggested_action", "changed_over_time", "confidence", "watch_for"],
		"properties": {
			"finding": {"type": "string", "description": "What you can see, in plain words"},
			"likely_cause": {"type": "string", "description": "Most likely cause, or say it is unclear"},
			"suggested_action": {"type": "string", "description": "One thing to do, or that nothing is needed"},
			"changed_over_time": {"type": "boolean", "description": "Whether the photographs show a change"},
			"confidence": {"type": "number", "description": "0 to 1; be honest when the images are poor"},
			"watch_for": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Signs that would confirm or rule this out"
			}
		}
	}`
	var schema map[string]any
	err := json.Unmarshal([]byte(raw), &schema)
	return schema, err
}
