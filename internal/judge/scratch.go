package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// scratchSystem answers about a plant nobody owns, so there is no record to
// reason from and the photograph is the whole of the evidence.
const scratchSystem = `You are answering a question about a plant the person
does not own and may never own: something photographed in a shop, in somebody
else's house, or growing outside.

There is no record for this plant. The photograph is all the evidence there is,
so say what you can see and be plain about what you cannot.

- Identify it if you can, and say how sure you are. "Some kind of philodendron,
  but I cannot tell which from this angle" is a better answer than a confident
  wrong species.
- Ask for a better photograph when one would settle it. Say exactly what to
  photograph: the undersides of the leaves, where the stem meets the soil, the
  whole plant rather than one leaf.
- If asked whether it is safe around an animal, answer from the species you
  actually identified, and say which species that is. Never answer a toxicity
  question from a common name alone: "lily" is six unrelated plants and two of
  them kill cats.
- Do not create a plant record. This is a question about something the person
  does not have, and filing it as one of their plants is wrong unless they ask
  for that in so many words.
- Talk like a person. Short, direct, no preamble.`

// Ask answers a question about a plant that is not in the collection.
// Deliberately not Consult: with no record to reason from, sharing that path
// would make the model reason about a row that is not there.
func (j *Judge) Ask(ctx context.Context, asked string, shown []Offer,
	prior []PriorAnswer, conversation uuid.UUID) (Answer, error) {
	if strings.TrimSpace(asked) == "" && len(shown) == 0 {
		return Answer{}, fmt.Errorf("nothing was asked and nothing was shown")
	}

	schema, err := answerSchema()
	if err != nil {
		return Answer{}, err
	}

	system := scratchSystem
	if j.acting != nil && j.acting.Sources != "" {
		system += "\n\n" + j.acting.Sources
	}

	// A photograph with no question is still a question, and the commonest one.
	opening := asked
	if strings.TrimSpace(opening) == "" {
		opening = "What is this, and is there anything I should know about it?"
	}

	var turns []Turn
	for _, turn := range prior {
		reply, err := json.Marshal(turn.Reply)
		if err != nil {
			continue
		}
		turns = append(turns, ask(text(turn.Asked)), answered(string(reply)))
	}
	turns = append(turns, ask(text(opening)))

	outcome, err := j.backend.Judge(ctx, Request{
		System:    system,
		Turns:     turns,
		Offered:   shown,
		Schema:    schema,
		MaxTokens: 2048,
		Session:   &Session{ID: conversation, Resuming: len(prior) > 0},
		Live:      true,
		Effort:    EffortMedium,
	})
	if err != nil {
		return Answer{}, err
	}

	var out Answer
	if err := json.Unmarshal([]byte(outcome.Answer), &out); err != nil {
		return Answer{}, fmt.Errorf("decode answer: %w", err)
	}
	out.Steps = outcome.Steps
	return out, nil
}
