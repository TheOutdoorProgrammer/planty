package judge

import (
	"fmt"
	"sort"
)

// Job is one of the questions Planty asks a model. Each asks for something
// different, which is why a model is chosen per job rather than once.
type Job string

const (
	JobAssess      Job = "assess"
	JobIdentify    Job = "identify"
	JobConsult     Job = "consult"
	JobAsk         Job = "ask"
	JobPostmortem  Job = "postmortem"
	JobOwnerUpdate Job = "owner_update"
)

// Jobs is every job, in the order a person reads them: the scheduled one
// first, then the ones they trigger themselves.
var Jobs = []Job{JobAssess, JobIdentify, JobConsult, JobAsk, JobPostmortem, JobOwnerUpdate}

// Skills are the things a job can require and a model can offer. One type for
// both sides so a requirement cannot be phrased differently from the ability
// that satisfies it.
type Skills struct {
	Vision        bool `json:"vision"`
	Schema        bool `json:"schema"`
	Tools         bool `json:"tools"`
	OfferedPhotos bool `json:"offered_photos"`
}

// Needs is what each job demands. Schema is universal because every call site
// unmarshals a validated answer; vision and tools follow the call sites that
// attach photographs and grant commands.
var Needs = map[Job]Skills{
	JobAssess:      {Schema: true},
	JobIdentify:    {Schema: true, Vision: true},
	JobConsult:     {Schema: true, Vision: true, Tools: true, OfferedPhotos: true},
	JobAsk:         {Schema: true, Vision: true, Tools: true, OfferedPhotos: true},
	JobPostmortem:  {Schema: true},
	JobOwnerUpdate: {Schema: true},
}

// Model is one selectable model: where it lives, what it can do, and how
// capable it is relative to the rest.
type Model struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`

	// Rank orders the catalogue smartest first, because a list sorted by name
	// tells nobody which to pick. Curated: no benchmark covers this whole set,
	// so it blends the vision evals that exist with each vendor's price tier.
	Rank int `json:"rank"`

	Skills Skills `json:"skills"`

	// Note explains a limit the picker would otherwise have to guess at.
	Note string `json:"note,omitempty"`
}

// Ref is how an assignment names a model.
func (m Model) Ref() string { return m.Provider + "/" + m.ID }

// CanDo reports whether a model satisfies a job, and why not when it does not.
// Enforced here rather than only filtered in the picker, so a stale client or
// a hand-written request cannot put a blind model on a job that shows photos.
func (m Model) CanDo(job Job) error {
	need, ok := Needs[job]
	if !ok {
		return fmt.Errorf("no such job %q", job)
	}
	if need.Vision && !m.Skills.Vision {
		return fmt.Errorf("%s cannot read images, and %s shows it photographs", m.Ref(), job)
	}
	if need.Schema && !m.Skills.Schema {
		return fmt.Errorf("%s cannot return a validated JSON answer, which every job requires", m.Ref())
	}
	if need.Tools && !m.Skills.Tools {
		return fmt.Errorf("%s cannot call tools, and %s lets the model run planty and read pages", m.Ref(), job)
	}
	if need.OfferedPhotos && !m.Skills.OfferedPhotos {
		return fmt.Errorf("%s cannot open offered historical photographs, which %s promises", m.Ref(), job)
	}
	return nil
}

// claudeSkills is what any Claude model reached through `claude -p` can do.
// The CLI supplies the tool loop, so tools come from the backend rather than
// from the model.
var claudeSkills = Skills{Vision: true, Schema: true, Tools: true, OfferedPhotos: true}

// known records observed capability, never advertised capability: gpt-5.6-luna
// accepts response_format, answers 200, and ignores it, so it is absent here.
// openai_live_test.go is what keeps this table honest.
var known = []Model{
	{Provider: "claude", ID: "claude-opus-5", Name: "Claude Opus 5", Rank: 1, Skills: claudeSkills},
	{Provider: "claude", ID: "claude-sonnet-5", Name: "Claude Sonnet 5", Rank: 2, Skills: claudeSkills},
	{Provider: "claude", ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Rank: 3, Skills: claudeSkills},

	{Provider: "opencode-go", ID: "kimi-k3", Name: "Kimi K3", Rank: 10,
		Skills: Skills{Vision: true, Schema: true, Tools: true, OfferedPhotos: true},
		Note: "Only reasons at maximum effort, and has the smallest request " +
			"allowance of any model here, so it occasionally refuses outright."},
	{Provider: "opencode-go", ID: "qwen3.8-max", Name: "Qwen3.8 Max", Rank: 11,
		Skills: Skills{Vision: true, Schema: true, Tools: true, OfferedPhotos: true},
		Note:   "Best measured at identifying things from a photograph."},
	{Provider: "opencode-go", ID: "glm-5.2", Name: "GLM-5.2", Rank: 14,
		Skills: Skills{Schema: true, Tools: true, OfferedPhotos: true}, Note: "Text only."},
	{Provider: "opencode-go", ID: "mimo-v2.5", Name: "MiMo-V2.5", Rank: 18,
		Skills: Skills{Vision: true, Schema: true, Tools: true, OfferedPhotos: true},
		Note:   "The cheapest model that can still see and return a validated answer."},
	{Provider: "opencode-go", ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", Rank: 19,
		Skills: Skills{Schema: true, Tools: true, OfferedPhotos: true}, Note: "Text only."},
}

// Catalog is every model the given providers can reach, smartest first. A
// provider that is not configured contributes nothing, so the picker never
// offers something the service cannot actually call.
func Catalog(configured map[string]Provider) []Model {
	var out []Model
	for _, m := range known {
		if _, ok := configured[m.Provider]; ok {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}

// Lookup finds one catalogue entry by provider and model id.
func Lookup(provider, id string) (Model, bool) {
	for _, m := range known {
		if m.Provider == provider && m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}
