package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MaxCandidates caps what comes back. A ranked list is honest; a list of
// fifteen is a way of saying nothing at all.
const MaxCandidates = 3

// Candidate is one ranked answer to what a plant is.
type Candidate struct {
	CommonName     string  `json:"common_name"`
	ScientificName string  `json:"scientific_name,omitempty"`
	Confidence     float64 `json:"confidence"`
}

// Sighting is where and when a photo was taken. Both are optional and both
// narrow the answer: a plant photographed outdoors in January in Ohio is a
// different candidate set from the same shape photographed in Florida.
type Sighting struct {
	TakenAt   *time.Time
	Latitude  *float64
	Longitude *float64
}

func identifySchema() (map[string]any, error) {
	raw := `{
		"type": "object",
		"additionalProperties": false,
		"required": ["candidates"],
		"properties": {
			"candidates": {
				"type": "array",
				"description": "Most likely first. An empty list beats a guessed name",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"required": ["common_name", "confidence"],
					"properties": {
						"common_name": {"type": "string"},
						"scientific_name": {"type": "string"},
						"confidence": {
							"type": "number",
							"description": "What you would bet, not how the picture feels"
						}
					}
				}
			}
		}
	}`
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		return nil, fmt.Errorf("identify schema: %w", err)
	}
	return schema, nil
}

// Identify names what a plant probably is, ranked. The caller has already cut
// it out of its background and confirmed it is a plant, so this is only asked
// about something worth asking.
func (j *Judge) Identify(ctx context.Context, image Frame, seen Sighting) ([]Candidate, error) {
	if len(image.Bytes) == 0 {
		return nil, fmt.Errorf("identify: no image")
	}

	schema, err := identifySchema()
	if err != nil {
		return nil, err
	}

	reply, err := j.backend.Judge(ctx, Request{
		System: identifySystem,
		Turns: []Turn{ask(
			picture(image.Media, image.Bytes),
			text(identifyPrompt(seen)),
		)},
		Schema:    schema,
		MaxTokens: 1024,
	})
	if err != nil {
		return nil, err
	}

	var answer struct {
		Candidates []Candidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(reply), &answer); err != nil {
		return nil, fmt.Errorf("decode candidates: %w", err)
	}
	if len(answer.Candidates) > MaxCandidates {
		answer.Candidates = answer.Candidates[:MaxCandidates]
	}
	return answer.Candidates, nil
}

const identifySystem = `You identify houseplants and garden plants from a photograph.

Return up to three candidates, most likely first, with a calibrated confidence
between 0 and 1. Confidence is what you would bet, not how the picture makes you
feel: a genus you are sure of and a species you are not is a low number, and
saying so is the useful answer.

Never invent a species to fill the list. One candidate is a fine answer, and an
empty list is better than a wrong name stated confidently, because somebody is
going to water a plant based on this.`

func identifyPrompt(seen Sighting) string {
	var b strings.Builder
	b.WriteString("What plant is this?")

	if seen.TakenAt != nil {
		fmt.Fprintf(&b, "\n\nPhotographed %s.", seen.TakenAt.Format("2 January 2006"))
	}
	if seen.Latitude != nil && seen.Longitude != nil {
		fmt.Fprintf(&b, "\nNear %.3f, %.3f.", *seen.Latitude, *seen.Longitude)
	}
	if seen.TakenAt != nil || seen.Latitude != nil {
		b.WriteString("\n\nUse the region and season to narrow it, but do not let " +
			"them override what is actually in the frame: an indoor plant can be " +
			"anywhere on earth in any month.")
	}
	return b.String()
}
