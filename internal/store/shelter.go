package store

import (
	"context"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Shelter marks plants as carried indoors, so the system can later tell you to
// put them back out.
func (s *Store) Shelter(ctx context.Context, slugs []string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE plants SET sheltered_at = now(), updated_at = now()
		WHERE slug = ANY($1) AND archived_at IS NULL AND sheltered_at IS NULL`, slugs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Unshelter marks plants as back outside.
func (s *Store) Unshelter(ctx context.Context, slugs []string) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE plants SET sheltered_at = NULL, updated_at = now()
		WHERE slug = ANY($1) AND archived_at IS NULL`, slugs)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ShelterAll marks every plant that has a cold threshold and is still outside.
// At dusk the real answer is "I brought them all in", not a list of slugs.
func (s *Store) ShelterAll(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE plants SET sheltered_at = now(), updated_at = now()
		WHERE archived_at IS NULL
		  AND status <> 'dead'
		  AND min_temp_f IS NOT NULL
		  AND sheltered_at IS NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// UnshelterAll puts everything back outside.
func (s *Store) UnshelterAll(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE plants SET sheltered_at = NULL, updated_at = now()
		WHERE archived_at IS NULL AND sheltered_at IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Sheltered returns plants currently indoors for cold, oldest first so the ones
// that have been stuck inside longest are named first.
func (s *Store) Sheltered(ctx context.Context) ([]plant.Plant, time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumns+`
		FROM plants
		WHERE archived_at IS NULL AND sheltered_at IS NOT NULL
		ORDER BY sheltered_at`)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()

	var out []plant.Plant
	for rows.Next() {
		p, err := scanPlant(rows)
		if err != nil {
			return nil, time.Time{}, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}

	var since time.Time
	if len(out) > 0 {
		if err := s.pool.QueryRow(ctx, `
			SELECT min(sheltered_at) FROM plants
			WHERE archived_at IS NULL AND sheltered_at IS NOT NULL`).Scan(&since); err != nil {
			return out, time.Time{}, err
		}
	}
	return out, since, nil
}
