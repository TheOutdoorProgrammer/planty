// Package seed loads the starting plant records, so a fresh database is useful
// rather than empty.
package seed

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/TheOutdoorProgrammer/planty/internal/store"
)

//go:embed friends.json
var friendsRaw []byte

type friendsFile struct {
	Steward   string        `json:"steward"`
	Plants    []plant.Plant `json:"plants"`
	Questions []string      `json:"questions"`
}

// Friends seeds the sabbatical plants and their open questions. Idempotent.
func Friends(ctx context.Context, s *store.Store, log *slog.Logger, steward string) error {
	var file friendsFile
	if err := json.Unmarshal(friendsRaw, &file); err != nil {
		return fmt.Errorf("parse seed: %w", err)
	}
	if steward == "" {
		steward = file.Steward
	}

	var added int
	for _, p := range file.Plants {
		if _, err := s.GetPlant(ctx, p.Slug); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return err
		}

		p.Steward = steward
		created, err := s.CreatePlant(ctx, p)
		if err != nil {
			return fmt.Errorf("create %s: %w", p.Slug, err)
		}
		log.Info("seeded plant", "slug", created.Slug, "steward", steward)
		added++
	}

	open, err := s.Questions(ctx, steward, plant.QuestionOpen)
	if err != nil {
		return err
	}
	if len(open) == 0 {
		for _, q := range file.Questions {
			if _, err := s.AskOwner(ctx, plant.Question{AskedOf: steward, Question: q}); err != nil {
				return fmt.Errorf("queue question: %w", err)
			}
		}
		log.Info("queued owner questions", "count", len(file.Questions))
	}

	log.Info("seed complete", "plants_added", added)
	return nil
}
