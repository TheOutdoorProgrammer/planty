// Package judge asks Claude what to do about a plant, given its record, its
// readings and what has been done to it lately.
package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/policy"
	"github.com/google/uuid"
)

// DefaultModel answers any job with no assignment of its own. Plant care is
// fuzzy reasoning over sparse, noisy evidence, which is where the capable
// model earns its cost.
const DefaultModel = "claude-opus-5"

// Judge turns evidence into a verdict.
type Judge struct {
	// backends is one per configured provider, so a job can be answered by a
	// different provider from the one next to it.
	backends map[string]Backend

	// fallback answers a job with no assignment, and is what Planty has always
	// done: the environment's choice of Claude.
	fallback Backend

	// assigned is consulted per call, so a model chosen on the phone takes
	// effect without restarting the service.
	assigned     Assignments
	instructions PromptInstructions

	acting *Acting
}

// Assignments names the model a job should use.
type Assignments interface {
	For(ctx context.Context, job Job) (Model, bool)
}

// Assigned attaches the store the assignments live in.
func (j *Judge) Assigned(a Assignments) *Judge {
	if j == nil {
		return nil
	}
	j.assigned = a
	return j
}

// dispatch sends one request to whichever backend answers for its job. An
// assignment naming a model the job cannot do is refused rather than attempted,
// because failing here names the misconfiguration instead of the symptom.
func (j *Judge) dispatch(ctx context.Context, req Request) (Outcome, error) {
	if j.instructions != nil {
		if overlay, ok := j.instructions.PromptInstructionsFor(ctx, req.Job); ok {
			req.System = withPromptInstructions(req.System, overlay)
		}
	}

	backend, model := j.fallback, ""

	if j.assigned != nil && req.Job != "" {
		if chosen, ok := j.assigned.For(ctx, req.Job); ok {
			if err := chosen.CanDo(req.Job); err != nil {
				return Outcome{}, err
			}
			picked, ok := j.backends[chosen.Provider]
			if !ok {
				return Outcome{}, fmt.Errorf("provider %q is not configured", chosen.Provider)
			}
			backend, model = picked, chosen.ID
		}
	}
	if backend == nil {
		return Outcome{}, fmt.Errorf("nothing can answer %s", req.Job)
	}
	if req.Job != "" {
		if gate, ok := backend.(interface{ CanDo(Job) error }); ok {
			if err := gate.CanDo(req.Job); err != nil {
				return Outcome{}, err
			}
		}
	}

	req.Model = model
	return backend.Judge(ctx, req)
}

// Able lets an explicitly acting job use the closed Planty command surface.
// Call sites still opt in per Request, so identification and reporting jobs do
// not inherit physical or record-writing authority from the shared Judge.
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
	cli, ok := j.fallback.(*cliBackend)
	if !ok {
		return ""
	}
	return cli.binary
}

// New builds a judge, or nil when nothing can answer, because the watering
// line has to keep running on a morning the model cannot be reached.
// PLANTY_JUDGE names the fallback backend; unset, an API key wins over the CLI.
func New() *Judge {
	fallback := backendFor(os.Getenv("PLANTY_JUDGE"))
	backends := map[string]Backend{}

	// A provider that cannot be built is left out rather than fatal: one
	// unreachable provider must not stop the others answering.
	if providers, err := Providers(); err == nil {
		for id, p := range providers {
			if built := backendForProvider(p); built != nil {
				backends[id] = built
			}
		}
	}
	if fallback == nil && len(backends) == 0 {
		return nil
	}
	return &Judge{backends: backends, fallback: fallback}
}

// Providers names what this judge can reach, for logs and error messages.
func (j *Judge) Providers() []string {
	out := make([]string, 0, len(j.backends))
	for id := range j.backends {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Backend names what answers a job with no assignment.
func (j *Judge) Backend() string {
	if j.fallback == nil {
		return "none"
	}
	return j.fallback.Name()
}

// DefaultSkills describes the unassigned backend. Assigned models carry their
// own catalogue skills; the default still needs to be honest in diagnostics
// and in the consultation UI.
func (j *Judge) DefaultSkills() Skills {
	if j == nil || j.fallback == nil {
		return Skills{}
	}
	switch j.fallback.(type) {
	case *cliBackend:
		return claudeSkills
	case *apiBackend:
		return Skills{Vision: true, Schema: true}
	case *openaiBackend:
		return Skills{Vision: true, Schema: true, Tools: true, OfferedPhotos: true}
	default:
		return Skills{}
	}
}

func backendForProvider(p Provider) Backend {
	switch p.Kind {
	case KindClaude:
		return cliIfInstalled(DefaultModel)
	case KindOpenAI:
		return newOpenAIBackend(p, "")
	default:
		return nil
	}
}

func backendFor(choice string) Backend {
	model := DefaultModel
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

	CurrentHealth     *plant.HealthEvent
	HealthEvidenceNew bool
	LatestPhoto       *PhotoEvidence
	PhotoCoverage     string
	PolicyDecisions   []policy.Evaluation
}

// PhotoEvidence is a photograph that is attached to the assessment request.
// Keeping its record id beside the bytes makes the resulting verdict able to
// prove exactly which image the model received.
type PhotoEvidence struct {
	ID      uuid.UUID
	TakenAt time.Time
	Caption string
	Frame   Frame
}

// SensorState is one probe's latest reading, expressed against its own
// baselines because two probes' absolute numbers are never comparable.
type SensorState struct {
	ReadingID  uuid.UUID
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

	HealthMode      plant.HealthMode `json:"health_mode"`
	HealthValue     float64          `json:"health_value"`
	HealthReasoning string           `json:"health_reasoning"`

	// Model is what answered. Filled in after decoding, not by it.
	Model string `json:"-"`

	// Attempts and the original malformed answer remain available to the run
	// ledger even when the bounded repair succeeds.
	Attempts       int    `json:"-"`
	OriginalError  string `json:"-"`
	OriginalOutput string `json:"-"`
}

// AssessmentError keeps the first malformed answer and the final failure
// together. Daily persists it against the plant instead of reducing a useful
// diagnosis to one aggregate failure count.
type AssessmentError struct {
	Model          string
	Attempts       int
	OriginalError  string
	OriginalOutput string
	FinalError     error
}

func (e *AssessmentError) Error() string {
	return fmt.Sprintf("repair verdict after %s: %v", e.OriginalError, e.FinalError)
}

func (e *AssessmentError) Unwrap() error { return e.FinalError }

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
- Health is an evidence-backed trend from 0 through 100, not a reward for a
  cronjob running. Use baseline only when health is unknown and record-backed
  evidence supports an absolute assessment. Use adjust only when evidence
  newer than the current score demonstrates a real change, and make its value
  an arbitrary signed delta. Otherwise use unchanged with value 0. A score of
  100 means no known or visible health deficit; 0 means confirmed dead, but it
  never archives or tells the system to discard the plant automatically.

Answer with the single most useful action and a one-sentence reason a beginner
can act on. No preamble, no hedging, no lists.`

const assessmentActingRules = `Rules for acting during this scheduled assessment:

- This assessment is about the plant whose slug is %q. An actuator command must
  name that plant, and may use only an actuator assigned to it.
- The complete reference also describes features reserved for conversations.
  This job's enforced command set is read-only garden inspection plus
  actuatorstart and actuatorstop; every other write is unavailable here.
- Read before writing. Use the plant record and list its actuators before any
  physical command. Do not create, rename, archive, or otherwise maintain plant
  records during a scheduled assessment.
- Watering remains recommendation-only here. Never run the water command from a
  scheduled assessment.
- You may start or stop an assigned fan when current, plant-specific evidence
  shows airflow is useful now. General species advice or a preventative routine
  is not enough. Use a fresh idempotency key, choose the shortest useful bounded
  duration, and never work around a refusal.
- The actuator list names every plant sharing a fan. Inspect each of those plant
  records before changing it. Do not stop an active bounded run merely because
  the plant currently being assessed does not need another run.
- A successful start automatically records airflow on every plant assigned to
  the shared fan. Do not log a second airflow observation.
- Perform any justified fan command before returning the required verdict JSON.
  The verdict must still describe the single most useful action for the person;
  mention completed airflow plainly so it is not presented as an undone chore.`

var assessmentAgentVerbs = []string{
	"plants", "show", "observations", "health", "actuators", "reminders",
	"sensors", "today", "questions", "coldwatch", "notes",
	"actuatorstart", "actuatorstop",
}

func assessmentActing(acting *Acting) *Acting {
	if acting == nil {
		return nil
	}
	scoped := *acting
	scoped.AgentVerbs = append([]string(nil), assessmentAgentVerbs...)
	return &scoped
}

// Assess asks for one plant's verdict.
func (j *Judge) Assess(ctx context.Context, e Evidence) (Result, error) {
	schema, err := resultSchema()
	if err != nil {
		return Result{}, err
	}

	parts := []Part{text(describe(e))}
	if e.LatestPhoto != nil {
		parts = append(parts, picture(e.LatestPhoto.Frame.Media, e.LatestPhoto.Frame.Bytes))
	}
	req := Request{
		Job:       JobAssess,
		System:    j.withActing(system, fmt.Sprintf(assessmentActingRules, e.Plant.Slug)),
		Turns:     []Turn{ask(parts...)},
		Schema:    schema,
		Acting:    assessmentActing(j.acting),
		MaxTokens: 2048,
		// One small judgment per plant per day, run many times over: medium is
		// where this model's quality holds without paying for depth.
		Effort: EffortMedium,
	}
	outcome, err := j.dispatch(ctx, req)
	if err != nil {
		return Result{}, err
	}

	out, invalid := decodeResult(outcome, e)
	if invalid == nil {
		out.Attempts = 1
		return out, nil
	}

	// One repair is enough to recover common truncation/schema mistakes without
	// letting a scheduled job loop or silently spend an unbounded amount.
	// Physical work already happened, if it was justified, so the repair pass is
	// deliberately schema-only and cannot start a second actuator command.
	req.Acting = nil
	req.System = system + `

This is a schema repair pass only. Do not act or claim a new action; return the
corrected verdict JSON for the assessment that already ran.`
	req.Turns = append(req.Turns,
		answered(outcome.Answer),
		ask(text("That answer was rejected: "+invalid.Error()+". Return one corrected JSON object matching the schema.")),
	)
	repaired, err := j.dispatch(ctx, req)
	if err != nil {
		return Result{}, &AssessmentError{
			Model: outcome.Model, Attempts: 2, OriginalError: invalid.Error(),
			OriginalOutput: outcome.Answer, FinalError: err,
		}
	}
	out, err = decodeResult(repaired, e)
	if err != nil {
		return Result{}, &AssessmentError{
			Model: repaired.Model, Attempts: 2, OriginalError: invalid.Error(),
			OriginalOutput: outcome.Answer, FinalError: err,
		}
	}
	out.Attempts = 2
	out.OriginalError = invalid.Error()
	out.OriginalOutput = outcome.Answer
	return out, nil
}

func decodeResult(outcome Outcome, evidence Evidence) (Result, error) {
	var out Result
	if err := json.Unmarshal([]byte(outcome.Answer), &out); err != nil {
		return Result{}, fmt.Errorf("decode verdict: %w", err)
	}
	if err := out.valid(evidence); err != nil {
		return Result{}, err
	}
	out.Model = outcome.Model
	return out, nil
}

func (r Result) valid(evidence Evidence) error {
	switch r.Action {
	case plant.ActionNone, plant.ActionWater, plant.ActionCheck, plant.ActionUrgent, plant.ActionHarvest:
	default:
		return fmt.Errorf("invalid verdict action %q", r.Action)
	}
	if strings.TrimSpace(r.Reasoning) == "" {
		return fmt.Errorf("verdict reasoning is empty")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("verdict confidence %.3f is outside 0 through 1", r.Confidence)
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("verdict sensor summary is empty")
	}
	if err := r.HealthMode.Valid(); err != nil {
		return err
	}
	if strings.TrimSpace(r.HealthReasoning) == "" {
		return fmt.Errorf("health reasoning is empty")
	}
	switch r.HealthMode {
	case plant.HealthUnchanged:
		if r.HealthValue != 0 {
			return fmt.Errorf("unchanged health must have value 0")
		}
	case plant.HealthBaseline:
		if evidence.CurrentHealth != nil {
			return fmt.Errorf("health already has a baseline")
		}
		if !evidenceHasReferences(evidence) {
			return fmt.Errorf("a health baseline needs record-backed evidence")
		}
		if r.HealthValue < 0 || r.HealthValue > 100 {
			return fmt.Errorf("health baseline %.3f is outside 0 through 100", r.HealthValue)
		}
	case plant.HealthAdjust:
		if evidence.CurrentHealth == nil {
			return fmt.Errorf("health needs a baseline before an adjustment")
		}
		if !evidence.HealthEvidenceNew {
			return fmt.Errorf("health cannot change without newer evidence")
		}
		if r.HealthValue == 0 || r.HealthValue < -100 || r.HealthValue > 100 {
			return fmt.Errorf("health delta %.3f must be non-zero and between -100 and 100", r.HealthValue)
		}
	}
	return nil
}

func evidenceHasReferences(e Evidence) bool {
	return len(e.Sensors)+len(e.Recent) > 0 || e.LatestPhoto != nil
}

func resultSchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["action", "reasoning", "confidence", "sensor_summary", "health_mode", "health_value", "health_reasoning"],
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
			},
			"health_mode": {
				"type": "string",
				"enum": ["unchanged", "baseline", "adjust"],
				"description": "Whether to leave health alone, establish its first absolute score, or apply a signed delta"
			},
			"health_value": {
				"type": "number",
				"description": "0 for unchanged, 0 through 100 for a baseline, or a non-zero signed delta for adjust"
			},
			"health_reasoning": {
				"type": "string",
				"description": "One sentence naming the evidence for the health decision"
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

	if e.CurrentHealth == nil {
		b.WriteString("\nHealth: unknown; no absolute baseline has been established.\n")
	} else {
		fmt.Fprintf(&b, "\nHealth: %.1f out of 100, assessed %s ago.\n",
			e.CurrentHealth.Score, ago(e.CurrentHealth.CreatedAt))
	}
	if e.HealthEvidenceNew {
		b.WriteString("Record-backed evidence is newer than that health assessment.\n")
	} else {
		b.WriteString("No record-backed evidence is newer than that health assessment; health must remain unchanged.\n")
	}
	if e.LatestPhoto != nil {
		fmt.Fprintf(&b, "Latest photograph: attached from %s ago", ago(e.LatestPhoto.TakenAt))
		if e.LatestPhoto.Caption != "" {
			fmt.Fprintf(&b, ", caption: %s", e.LatestPhoto.Caption)
		}
		b.WriteString(". Treat it as visual evidence and do not claim you did not see it.\n")
	} else {
		fmt.Fprintf(&b, "Latest photograph: not attached (%s). Do not make visual claims.\n", e.PhotoCoverage)
	}
	writePolicyContext(&b, e.PolicyDecisions)
	return b.String()
}

func writePolicyContext(b *strings.Builder, evaluations []policy.Evaluation) {
	if len(evaluations) == 0 {
		return
	}
	b.WriteString("\nOwner-authored OPA policy decisions:\n")
	for _, evaluation := range evaluations {
		fmt.Fprintf(b, "  %s policy: %s\n", evaluation.PolicyMode, evaluation.Decision.Summary)
		for _, fact := range evaluation.Decision.Agent.Facts {
			fmt.Fprintf(b, "    fact: %s\n", fact)
		}
		for _, guidance := range evaluation.Decision.Agent.Guidance {
			fmt.Fprintf(b, "    guidance: %s\n", guidance)
		}
		if evaluation.PolicyMode == policy.ModeEnforce {
			for _, action := range evaluation.Decision.Agent.DenyActions {
				fmt.Fprintf(b, "    denied action: %s\n", action)
			}
		}
	}
	b.WriteString("Policies may narrow decisions but never grant tools or override Planty's safety rules.\n")
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
