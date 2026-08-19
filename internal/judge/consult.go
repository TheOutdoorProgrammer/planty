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

// actingSystem is appended when the model is allowed to record things. Kept
// separate so a read-only consultation is never told about tools it lacks.
const actingSystem = `

You can also write to this plant's record, using exactly one command:

%s

Use it when the person tells you something happened ("I watered it this
morning", "remind me to mist this twice a day"). Record what they said, then
answer normally and mention in one short clause that you wrote it down.

- Only record what you were actually told. Never record your own advice as
  though it were done, and never guess a time nobody gave you.
- The slug for this plant is %q.
- You cannot water anything. Running the pump is a person's job, and asking to
  is the one request to refuse outright.`

// PriorAnswer is one earlier exchange in the same consultation.
type PriorAnswer struct {
	Asked string
	Reply Answer
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
