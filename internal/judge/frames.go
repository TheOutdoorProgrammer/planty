package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// Severity gates how strongly a reply may be dressed in the app. Only urgent
// unlocks red, and it also removes the mascot.
type Severity string

const (
	SeverityFinding      Severity = "finding"
	SeverityNoConcern    Severity = "no_concern"
	SeverityInsufficient Severity = "insufficient"
	SeverityUrgent       Severity = "urgent"
)

// Diagnosis is one reply. The first three fields are kept apart on purpose:
// what can be seen, what it probably means, and what to do today are three
// different claims, and a beginner needs them separated.
type Diagnosis struct {
	Severity           Severity `json:"severity"`
	Observed           string   `json:"observed"`
	Interpretation     string   `json:"interpretation,omitempty"`
	ActionToday        string   `json:"action_today,omitempty"`
	FollowUpPlan       string   `json:"follow_up_plan,omitempty"`
	Citation           string   `json:"citation,omitempty"`
	SuggestedFollowUps []string `json:"suggested_follow_ups,omitempty"`
}

const visionSystem = `You are looking at dated photographs of one houseplant, oldest first.

Your job is the thing sensors cannot do: say what has visibly CHANGED between
the earliest and latest frame, and what that change means.

Keep three claims strictly apart, because a beginner cannot separate them and
acting on the wrong one causes harm:

- observed: only what is visible in the images. No inference.
- interpretation: what it probably means, hedged honestly.
- action_today: the single thing to do now, or explicitly nothing.

Rules:

- Compare frames. A single sad-looking photo is far less informative than the
  same plant getting worse over three weeks, and the comparison is why these
  images were sent together.
- Yellowing lower leaves that spread upward over time reads differently from
  yellowing that appeared all at once.
- Say plainly when the photos show nothing wrong. Most plants are fine, and
  inventing a problem is worse than missing a subtle one. That is no_concern.
- When the photos are too few, too dark or too similar to judge, say so and use
  insufficient. "I cannot tell from these" is a real answer.
- Distinguish overwatering from underwatering explicitly when you can. They look
  similar to a beginner and the corrective actions are opposite, so guessing
  wrong actively causes harm.
- Reserve urgent for visible evidence of active harm, not for a worry.
- citation names what you actually looked at, so the reader can weigh it.
- suggested_follow_ups are questions the reader might sensibly ask next.

One clear finding. No lists of possibilities.`

// PriorTurn is one earlier exchange in the same diagnosis conversation.
type PriorTurn struct {
	Asked string
	Reply Diagnosis
}

// Diagnose reads a plant's photo timeline and answers one question about it.
// Prior turns are replayed so a follow-up can refer to the first answer.
func (j *Judge) Diagnose(ctx context.Context, p plant.Plant, frames []Frame, asked string, prior []PriorTurn) (Diagnosis, error) {
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

	opening := []Part{text(visionPreamble(p, frames))}
	for _, f := range frames {
		opening = append(opening,
			text(fmt.Sprintf("Taken %s ago%s:", ago(f.TakenAt), caption(f))),
			picture(f.Media, f.Bytes),
		)
	}
	if asked == "" {
		asked = "What do these photographs show, and what should I do about it?"
	}
	opening = append(opening, text(asked))

	// The images ride on the first turn only; later turns refer back to them.
	turns := []Turn{ask(opening...)}
	if len(prior) > 0 {
		turns = replay(prior, opening, asked)
	}

	outcome, err := j.backend.Judge(ctx, Request{
		System:    visionSystem,
		Turns:     turns,
		Schema:    schema,
		MaxTokens: 2048,
		// Comparing frames over time is the hard part; worth the depth.
		Effort: EffortHigh,
	})
	if err != nil {
		return Diagnosis{}, err
	}

	var out Diagnosis
	if err := json.Unmarshal([]byte(outcome.Answer), &out); err != nil {
		return Diagnosis{}, fmt.Errorf("decode diagnosis: %w", err)
	}
	return out, nil
}

// replay rebuilds the conversation: the photographs and the original question
// first, then each earlier answer, then what is being asked now.
func replay(prior []PriorTurn, opening []Part, asked string) []Turn {
	// The opening user turn already carries the images; swap its trailing
	// question for the one that was actually asked first.
	restored := append([]Part{}, opening[:len(opening)-1]...)
	restored = append(restored, text(prior[0].Asked))

	turns := []Turn{ask(restored...)}
	for i, turn := range prior {
		reply, err := json.Marshal(turn.Reply)
		if err != nil {
			continue
		}
		turns = append(turns, answered(string(reply)))

		if i+1 < len(prior) {
			turns = append(turns, ask(text(prior[i+1].Asked)))
		}
	}
	return append(turns, ask(text(asked)))
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

// describePot reports the drainage hole independently of the other pot
// details, because a pot with no hole is the most common way a plant drowns
// and it used to be dropped whenever nobody recorded the material.
func describePot(p plant.Plant) string {
	var parts []string
	if p.PotSizeIn != nil {
		parts = append(parts, fmt.Sprintf("%.0f inch", *p.PotSizeIn))
	}
	if p.PotMaterial != "" {
		parts = append(parts, p.PotMaterial)
	}

	described := ""
	if len(parts) > 0 {
		described = "Pot: " + strings.Join(parts, " ")
	}

	if p.HasDrainage != nil && !*p.HasDrainage {
		if described == "" {
			return "Pot has NO drainage hole, so water cannot leave it."
		}
		return described + ", with NO drainage hole, so water cannot leave it."
	}
	if described == "" {
		return ""
	}
	return described + "."
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
		"required": ["severity", "observed", "interpretation", "action_today", "follow_up_plan", "citation", "suggested_follow_ups"],
		"properties": {
			"severity": {
				"type": "string",
				"enum": ["finding", "no_concern", "insufficient", "urgent"],
				"description": "urgent only for visible active harm; insufficient when the images cannot support a judgment"
			},
			"observed": {"type": "string", "description": "Only what is visible. No inference"},
			"interpretation": {"type": "string", "description": "What it probably means, hedged honestly"},
			"action_today": {"type": "string", "description": "The one thing to do now, or explicitly nothing"},
			"follow_up_plan": {"type": "string", "description": "What to check next and when"},
			"citation": {"type": "string", "description": "What you actually looked at"},
			"suggested_follow_ups": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Questions the reader might sensibly ask next"
			}
		}
	}`
	var schema map[string]any
	err := json.Unmarshal([]byte(raw), &schema)
	return schema, err
}
