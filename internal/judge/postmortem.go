package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// History is everything a dead plant left behind.
type History struct {
	Plant        plant.Plant
	Observations []plant.Observation
	Verdicts     []plant.Verdict
	Readings     []Sample
	PhotoNotes   []string
}

// Sample is one soil reading in the run-up, already expressed as a fraction of
// that probe's own range.
type Sample struct {
	TakenAt  time.Time
	Fraction float64
}

// Autopsy is what the plant taught.
type Autopsy struct {
	LikelyCause string `json:"likely_cause"`
	Narrative   string `json:"narrative"`
	Lesson      string `json:"lesson"`
	Preventable bool   `json:"was_preventable"`
}

const postmortemSystem = `A plant has died. Work out what most likely killed it.

You are writing for someone learning to keep plants alive, who kept this record
specifically so that a death would teach him something. Be direct about the
cause even when the cause was him.

- Overwatering and underwatering both end in droop, yellowing and collapse, and
  a beginner cannot tell them apart. If the record can tell them apart, say
  which it was and what in the record shows it.
- Look at what the readings did in the weeks before, not just at the end. The
  fatal mistake is usually weeks upstream of the symptom.
- A gap in the record is itself evidence: a plant nobody looked at for a month
  probably died of not being looked at.
- If the record genuinely cannot say, say that. A confident wrong cause teaches
  the wrong lesson, which is worse than no lesson.

The lesson should be one sentence that would change what he does next time,
specific to this plant and this record. Not general plant advice.`

// Postmortem asks what killed a plant.
func (j *Judge) Postmortem(ctx context.Context, h History) (Autopsy, error) {
	schema, err := autopsySchema()
	if err != nil {
		return Autopsy{}, err
	}

	message, err := j.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(j.model),
		MaxTokens: 3072,
		System:    []anthropic.TextBlockParam{{Text: postmortemSystem}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(narrate(h))),
		},
		OutputConfig: anthropic.OutputConfigParam{
			// Runs once per death and the answer is the whole point of the record.
			Effort: anthropic.OutputConfigEffortHigh,
			Format: anthropic.JSONOutputFormatParam{Schema: schema},
		},
	})
	if err != nil {
		return Autopsy{}, err
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return Autopsy{}, ErrRefused
	}

	for _, block := range message.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			var out Autopsy
			if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
				return Autopsy{}, fmt.Errorf("decode autopsy: %w", err)
			}
			return out, nil
		}
	}
	return Autopsy{}, fmt.Errorf("no autopsy in response")
}

func narrate(h History) string {
	var b strings.Builder
	p := h.Plant

	fmt.Fprintf(&b, "%s", p.CommonName)
	if p.BotanicalName != "" {
		fmt.Fprintf(&b, " (%s)", p.BotanicalName)
	}
	b.WriteString(" is dead.\n\n")

	if p.AcquiredAt != nil {
		fmt.Fprintf(&b, "Acquired %s ago.\n", ago(*p.AcquiredAt))
	}
	fmt.Fprintf(&b, "Kept in %s, %s light, watered by %s, reaching it was %s.\n",
		orUnknown(p.Location), orUnknown(string(p.LightExposure)),
		p.WateringMethod, p.Accessibility)

	if pot := describePot(p); pot != "" {
		fmt.Fprintf(&b, "%s\n", pot)
	}
	if p.CareProfile.OwnerSays != "" {
		fmt.Fprintf(&b, "The owner had said: %q\n", p.CareProfile.OwnerSays)
	}

	b.WriteString("\nSoil readings, as a fraction of that probe's own dry-to-wet range:\n")
	if len(h.Readings) == 0 {
		b.WriteString("  none were ever recorded\n")
	}
	for _, s := range h.Readings {
		fmt.Fprintf(&b, "  %s ago: %.0f%%\n", ago(s.TakenAt), s.Fraction*100)
	}

	b.WriteString("\nWhat was done to it:\n")
	if len(h.Observations) == 0 {
		b.WriteString("  nothing was ever recorded\n")
	}
	for _, o := range h.Observations {
		fmt.Fprintf(&b, "  %s ago: %s", ago(o.OccurredAt), o.Kind)
		if o.Body != "" {
			fmt.Fprintf(&b, " - %s", o.Body)
		}
		fmt.Fprintf(&b, " (recorded by %s)\n", o.Source)
	}

	if len(h.Verdicts) > 0 {
		b.WriteString("\nWhat Planty advised, and whether it was acknowledged:\n")
		for _, v := range h.Verdicts {
			ack := "not acknowledged"
			if v.AcknowledgedAt != nil {
				ack = "acknowledged"
			}
			fmt.Fprintf(&b, "  %s ago: %s - %s (%s)\n",
				ago(v.ForDate), v.Action, v.Reasoning, ack)
		}
	}

	if len(h.PhotoNotes) > 0 {
		b.WriteString("\nWhat the photographs showed over time:\n")
		for _, note := range h.PhotoNotes {
			fmt.Fprintf(&b, "  %s\n", note)
		}
	}
	return b.String()
}

func autopsySchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["likely_cause", "narrative", "lesson", "was_preventable"],
		"properties": {
			"likely_cause": {"type": "string", "description": "A short phrase, or that the record cannot say"},
			"narrative": {"type": "string", "description": "What the record shows happened, in order"},
			"lesson": {"type": "string", "description": "One sentence that changes what he does next time"},
			"was_preventable": {"type": "boolean", "description": "Whether the record suggests it could have been saved"}
		}
	}`
	var schema map[string]any
	err := json.Unmarshal([]byte(raw), &schema)
	return schema, err
}
