package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// OwnerPlantWeek is the small, owner-relevant slice of one plant's record used
// to write a seven-day update. LatestPhoto is metadata only; the actual image is
// attached to Messages by iOS after the summary has been written.
type OwnerPlantWeek struct {
	Plant        plant.Plant
	Observations []plant.Observation
	Verdicts     []plant.Verdict
	LatestPhoto  *plant.Photo
}

const ownerUpdateSystem = `Write a short text-message update to the owner of plants someone else is caring for.

The recipient already knows their plants. Summarize only the last seven days: what was done, anything that changed, anything Planty noticed, and whether anything currently needs concern. Be warm and factual, not cute or alarmist. Do not mention internal sensor IDs, confidence scores, AI, Claude, prompts, or database details. Do not invent an observation that is not in the record. If the week was uneventful, say that plainly. Name each plant at least once. Write 80-180 words as ready-to-send message prose with no heading or bullet list.`

// OwnerUpdate turns a week of records into prose that can be sent unchanged.
func (j *Judge) OwnerUpdate(ctx context.Context, steward string, week []OwnerPlantWeek) (string, error) {
	if j == nil {
		return "", fmt.Errorf("owner update needs a model")
	}
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"summary"},
		"properties": map[string]any{
			"summary": map[string]any{"type": "string"},
		},
	}
	outcome, err := j.dispatch(ctx, Request{
		Job:    JobOwnerUpdate,
		System: ownerUpdateSystem,
		Turns:  []Turn{ask(text(describeOwnerWeek(steward, week)))},
		Schema: schema, MaxTokens: 1200, Effort: EffortMedium,
	})
	if err != nil {
		return "", err
	}
	var decoded struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(outcome.Answer), &decoded); err != nil {
		return "", fmt.Errorf("decode owner update: %w", err)
	}
	if strings.TrimSpace(decoded.Summary) == "" {
		return "", fmt.Errorf("owner update was empty")
	}
	return strings.TrimSpace(decoded.Summary), nil
}

func describeOwnerWeek(steward string, week []OwnerPlantWeek) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Owner: %s\nWindow: the previous seven days\n", steward)
	for _, record := range week {
		p := record.Plant
		fmt.Fprintf(&b, "\nPlant: %s", p.CommonName)
		if p.BotanicalName != "" {
			fmt.Fprintf(&b, " (%s)", p.BotanicalName)
		}
		fmt.Fprintf(&b, "\nCurrent status: %s\n", p.Status)
		if p.CareProfile.OwnerSays != "" {
			fmt.Fprintf(&b, "Owner instructions: %s\n", p.CareProfile.OwnerSays)
		}
		if len(record.Observations) == 0 {
			b.WriteString("Recorded care/events: none\n")
		} else {
			b.WriteString("Recorded care/events:\n")
			for _, observation := range record.Observations {
				fmt.Fprintf(&b, "- %s: %s", observation.OccurredAt.Format("Jan 2"), observation.Kind)
				if observation.Body != "" {
					fmt.Fprintf(&b, " — %s", observation.Body)
				}
				b.WriteString("\n")
			}
		}
		if len(record.Verdicts) > 0 {
			b.WriteString("Planty conclusions:\n")
			for _, verdict := range record.Verdicts {
				fmt.Fprintf(&b, "- %s: %s — %s\n", verdict.ForDate.Format("Jan 2"), verdict.Action, verdict.Reasoning)
			}
		}
		if record.LatestPhoto != nil {
			photo := record.LatestPhoto
			fmt.Fprintf(&b, "Latest photo: %s", photo.TakenAt.Format(time.RFC3339))
			if photo.Caption != "" {
				fmt.Fprintf(&b, ", caption: %s", photo.Caption)
			}
			if photo.VisionFindings != "" {
				fmt.Fprintf(&b, ", visual findings: %s", photo.VisionFindings)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
