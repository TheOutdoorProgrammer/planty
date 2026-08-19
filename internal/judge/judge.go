// Package judge asks Claude what to do about a plant, given its record, its
// readings and what has been done to it lately.
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Model is the judgment model. Plant care is fuzzy reasoning over sparse,
// noisy evidence, which is where the capable model earns its cost.
const Model = "claude-opus-5"

// Judge turns evidence into a verdict.
type Judge struct {
	backend Backend
	acting  *Acting
}

// Able lets a judge write to a plant's record as well as read it. Only the
// conversation is given this: a daily verdict that quietly recorded
// observations would be judging evidence it had written itself.
func (j *Judge) Able(a *Acting) *Judge {
	if j == nil {
		return nil
	}
	j.acting = a
	return j
}

// CLIBinary is the command the model would be allowed to run, empty when this
// judge does not shell out to anything.
func (j *Judge) CLIBinary() string {
	cli, ok := j.backend.(*cliBackend)
	if !ok {
		return ""
	}
	return cli.binary
}

// New builds a judge, or nil when nothing can answer, because the watering
// line has to keep running on a morning the model cannot be reached.
// PLANTY_JUDGE names the backend; unset, an API key wins over the CLI.
func New() *Judge {
	backend := backendFor(os.Getenv("PLANTY_JUDGE"))
	if backend == nil {
		return nil
	}
	return &Judge{backend: backend}
}

// Backend names which of the two answered, for logs and error messages.
func (j *Judge) Backend() string { return j.backend.Name() }

func backendFor(choice string) Backend {
	model := Model
	if override := os.Getenv("PLANTY_JUDGE_MODEL"); override != "" {
		model = override
	}
	key := os.Getenv("ANTHROPIC_API_KEY")

	switch choice {
	case "api":
		if key == "" {
			return nil
		}
		return newAPIBackend(key, model)
	case "cli":
		return cliIfInstalled(model)
	}

	if key != "" {
		return newAPIBackend(key, model)
	}
	return cliIfInstalled(model)
}

func cliIfInstalled(model string) Backend {
	binary := os.Getenv("PLANTY_CLAUDE_BIN")
	if binary == "" {
		found, err := exec.LookPath("claude")
		if err != nil {
			return nil
		}
		binary = found
	}
	return newCLIBackend(binary, model)
}

// Evidence is everything known about one plant at judgment time.
type Evidence struct {
	Plant        plant.Plant
	Sensors      []SensorState
	Recent       []plant.Observation
	LastWatered  *time.Time
	AmbientTempF *float64
	AmbientRH    *float64
	Season       string
}

// SensorState is one probe's latest reading, expressed against its own
// baselines because two probes' absolute numbers are never comparable.
type SensorState struct {
	Role       plant.SensorRole
	Raw        float64
	Fraction   *float64
	Calibrated bool
	TakenAt    time.Time
}

// Result is the model's answer, shaped to fit a plant.Verdict.
type Result struct {
	Action     plant.Action `json:"action"`
	Reasoning  string       `json:"reasoning"`
	Confidence float64      `json:"confidence"`
	Summary    string       `json:"sensor_summary"`
}

// ErrRefused reports that safety classifiers declined the request.
var ErrRefused = fmt.Errorf("model declined the request")

const system = `You decide what a beginner should do about one houseplant today.

You are advising someone who has openly said he does not know what he is doing,
and who is looking after a friend's plants as well as his own. He needs one
clear instruction, not a lesson.

Rules that matter more than anything else you know about plants:

- Overwatering kills far more houseplants than drought. When the evidence is
  ambiguous, waiting a day is nearly always safer than watering.
- An uncalibrated soil sensor is not evidence. Say so rather than guessing.
- The owner's own instructions outrank general species advice. If a care
  profile records what the owner said, follow it.
- A plant that is hard to reach and watered by hand is the one most likely to
  be forgotten. Escalate those sooner.
- Say "none" freely. Most days most plants need nothing, and a system that
  invents chores gets ignored, which is how plants actually die.

Answer with the single most useful action and a one-sentence reason a beginner
can act on. No preamble, no hedging, no lists.`

// Assess asks for one plant's verdict.
func (j *Judge) Assess(ctx context.Context, e Evidence) (Result, error) {
	schema, err := resultSchema()
	if err != nil {
		return Result{}, err
	}

	answer, err := j.backend.Judge(ctx, Request{
		System:    system,
		Turns:     []Turn{ask(text(describe(e)))},
		Schema:    schema,
		MaxTokens: 2048,
		// One small judgment per plant per day, run many times over: medium is
		// where this model's quality holds without paying for depth.
		Effort: EffortMedium,
	})
	if err != nil {
		return Result{}, err
	}

	var out Result
	if err := json.Unmarshal([]byte(answer), &out); err != nil {
		return Result{}, fmt.Errorf("decode verdict: %w", err)
	}
	return out, nil
}

func resultSchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["action", "reasoning", "confidence", "sensor_summary"],
		"properties": {
			"action": {
				"type": "string",
				"enum": ["none", "water", "check", "urgent", "harvest"],
				"description": "The single action to take, or none"
			},
			"reasoning": {
				"type": "string",
				"description": "One sentence a beginner can act on"
			},
			"confidence": {
				"type": "number",
				"description": "0 to 1; be honest when evidence is thin"
			},
			"sensor_summary": {
				"type": "string",
				"description": "What the readings showed, in plain words"
			}
		}
	}`
	var schema map[string]any
	err := json.Unmarshal([]byte(raw), &schema)
	return schema, err
}

// describe renders the evidence as prose, because a model reads a paragraph
// about a plant better than it reads a struct dump.
func describe(e Evidence) string {
	var b strings.Builder
	p := e.Plant

	fmt.Fprintf(&b, "Plant: %s", p.CommonName)
	if p.BotanicalName != "" {
		fmt.Fprintf(&b, " (%s)", p.BotanicalName)
	}
	fmt.Fprintf(&b, "\nKept as: %s\n", p.Domain)

	if p.IsFriends() {
		fmt.Fprintf(&b, "Belongs to %s, not to the person caring for it. Stakes are higher.\n", p.Steward)
	}
	fmt.Fprintf(&b, "Location: %s. Reaching it is %s.\n", p.Location, p.Accessibility)

	switch p.WateringMethod {
	case plant.WateringLetPot:
		fmt.Fprintf(&b, "Watered automatically by the LetPot line.\n")
	default:
		fmt.Fprintf(&b, "Watered by hand, so nothing happens unless a person does it.\n")
	}

	if pot := describePot(p); pot != "" {
		fmt.Fprintf(&b, "%s\n", pot)
	}
	if p.LightExposure != "" {
		fmt.Fprintf(&b, "Light it gets: %s.\n", p.LightExposure)
	}
	if p.CareProfile.WantsLight != "" {
		fmt.Fprintf(&b, "Light it wants: %s.\n", p.CareProfile.WantsLight)
	}
	if p.CareProfile.OwnerSays != "" {
		fmt.Fprintf(&b, "The owner said: %q\n", p.CareProfile.OwnerSays)
	}
	if p.CareProfile.Quirks != "" {
		fmt.Fprintf(&b, "Known quirks: %s\n", p.CareProfile.Quirks)
	}
	if p.CareProfile.NeedsPollination != nil && *p.CareProfile.NeedsPollination {
		fmt.Fprintf(&b, "Needs pollination; indoors that means airflow or hand pollination or it sets no fruit.\n")
	}

	b.WriteString("\nReadings:\n")
	if len(e.Sensors) == 0 {
		b.WriteString("  none, this plant has no sensor\n")
	}
	for _, s := range e.Sensors {
		if !s.Calibrated {
			fmt.Fprintf(&b, "  %s: raw %.1f, NOT CALIBRATED so not trustworthy (%s ago)\n",
				s.Role, s.Raw, ago(s.TakenAt))
			continue
		}
		fmt.Fprintf(&b, "  %s: %.0f%% between its own dry and wet marks (%s ago)\n",
			s.Role, *s.Fraction*100, ago(s.TakenAt))
	}
	if e.AmbientTempF != nil {
		fmt.Fprintf(&b, "  ambient: %.0fF", *e.AmbientTempF)
		if e.AmbientRH != nil {
			fmt.Fprintf(&b, ", %.0f%% humidity", *e.AmbientRH)
		}
		b.WriteString("\n")
	}

	if e.LastWatered != nil {
		fmt.Fprintf(&b, "\nLast watered: %s ago.\n", ago(*e.LastWatered))
	} else {
		b.WriteString("\nNo watering has ever been recorded for this plant.\n")
	}
	if p.MinTempF != nil {
		fmt.Fprintf(&b, "Needs protecting below %.0fF.\n", *p.MinTempF)
	}
	if e.Season != "" {
		fmt.Fprintf(&b, "Season: %s.\n", e.Season)
	}

	if len(e.Recent) > 0 {
		b.WriteString("\nRecently:\n")
		for _, o := range e.Recent {
			fmt.Fprintf(&b, "  %s ago, %s", ago(o.OccurredAt), o.Kind)
			if o.Body != "" {
				fmt.Fprintf(&b, ": %s", o.Body)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
