package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ConsultWindow is how far back a conversation reads. Long enough to show a
// season turning and a watering rhythm, short enough that a plant's whole life
// is not re-read to answer "are these leaves normal".
const ConsultWindow = 45 * 24 * time.Hour

// Answer is a reply to a question about a plant. Deliberately shaped like a
// person talking rather than a verdict: this is a conversation, and the daily
// judgment is what produces actions.
type Answer struct {
	Reply       string   `json:"reply"`
	Confidence  float64  `json:"confidence"`
	LookedAt    string   `json:"looked_at,omitempty"`
	Suggestions []string `json:"suggested_follow_ups,omitempty"`
}

const consultSystem = `You are answering a question about one specific houseplant,
for the beginner who looks after it.

You have its record: what has been done to it, what its probes have read, and
what earlier photographs of it showed. Answer from that record, not from
general plant advice, whenever the record can answer at all.

- Answer the question asked. Do not turn every question into a care lecture.
- Use the record. "You watered it four days ago and it is still at 70%" is
  worth more than anything true of pothos in general.
- Say when the record cannot answer, and say what would answer it. A gap is
  information: a plant nobody has looked at for three weeks is a fact.
- Photographs may be available to open. Only look if seeing one would change
  your answer, and say which you looked at if you did.
- Overwatering kills more houseplants than drought. When it is ambiguous,
  waiting is nearly always the safer advice.
- Talk like a person. Short, direct, no preamble and no lists of possibilities.`

// actingSystem is appended only when the model may act, so a read-only
// consultation never hears about tools it lacks. %s is agent.Usage, carried
// in via Acting because importing agent from here is a cycle; %q is the slug.
const actingSystem = `

You can also act, through one command and nothing else: planty agent. What
follows is its complete reference — every verb, every flag, every valid value,
with an example each. Build commands straight from this text; never run a help
command to explore, and never reach for any other program.

%s

Rules for acting, which outrank everything above:

- This conversation is about the plant whose slug is %q. Act on that plant
  unless the person plainly names a different one.
- Record only what the person tells you actually happened, when they say it
  happened. Never record your own advice as though it were done, and never
  guess a time nobody gave you: with no time given, leave --when off and it
  is recorded as now.
- Watering is real. The water verb runs the physical pump on the LetPot line,
  after checking the probes. Run it when the person asks you to water, and
  never on your own initiative: suggesting it is fine, running it unasked is
  not. If it declines to pump, relay its reason.
- Asked whether a plant is dangerous to an animal or a person, answer from its
  stored toxicity if it has one. If it does not, look it up on the trusted
  sources and record what you find with the toxicity verb, so the next person
  to ask gets an answer without spending anything. Rate from the botanical
  name, never from the common one, and if you cannot establish the species,
  say so instead of rating it.
- Never answer a toxicity question with reassurance you have not earned.
  "Nobody has checked this one" is a real answer; "probably fine" is not.

- Act first, then answer normally, mentioning in one short clause what you
  wrote down or did. If a command is refused, pass its reason on in plain
  words rather than retrying blind.`

// PriorAnswer is one earlier exchange in the same consultation.
type PriorAnswer struct {
	Asked string
	Reply Answer

	// Set when a photograph was attached to that turn, so replaying a
	// conversation after its model session is gone can hand the pictures over
	// again rather than answering as though they were never sent.
	PhotoID *uuid.UUID
}

// Consult answers a question about a plant from its record, photographs
// offered rather than attached. The conversation's id doubles as the model
// session's, so a follow-up continues rather than re-reading everything.
func (j *Judge) Consult(ctx context.Context, h History, offered []Offer,
	asked string, prior []PriorAnswer, conversation uuid.UUID) (Answer, error) {
	if strings.TrimSpace(asked) == "" {
		return Answer{}, fmt.Errorf("no question was asked")
	}

	schema, err := answerSchema()
	if err != nil {
		return Answer{}, err
	}

	system := consultSystem
	if j.acting != nil {
		system += fmt.Sprintf(actingSystem, j.acting.Usage, h.Plant.Slug)
		if j.acting.Sources != "" {
			system += "\n\n" + j.acting.Sources
		}
	}

	turns := []Turn{ask(text(record(h, ongoing)))}
	for _, turn := range prior {
		reply, err := json.Marshal(turn.Reply)
		if err != nil {
			continue
		}
		turns = append(turns, ask(text(turn.Asked)), answered(string(reply)))
	}
	turns = append(turns, ask(text(asked)))

	reply, err := j.backend.Judge(ctx, Request{
		System:    system,
		Turns:     turns,
		Offered:   offered,
		Schema:    schema,
		MaxTokens: 2048,
		Session:   &Session{ID: conversation, Resuming: len(prior) > 0},
		Acting:    j.acting,
		// A conversation is answered rather than deliberated over, and a slow
		// reply to "is this normal" is a worse answer than a quick one.
		Effort: EffortMedium,
	})
	if err != nil {
		return Answer{}, err
	}

	var out Answer
	if err := json.Unmarshal([]byte(reply), &out); err != nil {
		return Answer{}, fmt.Errorf("decode answer: %w", err)
	}
	return out, nil
}

// Offers labels photographs by when they were taken, which is the only thing
// the model needs to decide whether one is worth opening.
func Offers(shots []plant.Photo, bytesFor func(plant.Photo) ([]byte, string, bool)) []Offer {
	out := make([]Offer, 0, len(shots))
	for _, shot := range shots {
		raw, media, ok := bytesFor(shot)
		if !ok {
			continue
		}
		label := fmt.Sprintf("%s, %s ago", shot.TakenAt.Format("2 January"), ago(shot.TakenAt))
		if shot.Caption != "" {
			label += " (" + shot.Caption + ")"
		}
		out = append(out, Offer{Label: label, Media: media, Bytes: raw})
	}
	return out
}

func answerSchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["reply", "confidence", "looked_at", "suggested_follow_ups"],
		"properties": {
			"reply": {
				"type": "string",
				"description": "The answer to what was asked, in plain words"
			},
			"confidence": {
				"type": "number",
				"description": "0 to 1; be honest when the record is thin"
			},
			"looked_at": {
				"type": "string",
				"description": "Which photographs you opened, or empty if none"
			},
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
