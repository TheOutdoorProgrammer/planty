package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// Chaseable returns unacknowledged actionable verdicts old enough to chase and
// not yet chased to the cap, most at risk of neglect first.
func (s *Store) Chaseable(ctx context.Context, olderThan time.Duration, maxEscalations int) ([]plant.DigestEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumnsFor("p")+`,
		       v.id, v.plant_id, v.for_date, v.action, v.reasoning,
		       v.confidence, v.evidence, v.created_at, v.acknowledged_at,
		       v.escalations
		FROM verdicts v
		JOIN plants p ON p.id = v.plant_id
		WHERE v.acknowledged_at IS NULL
		  AND v.action <> 'none'
		  AND p.archived_at IS NULL
		  AND v.escalations < $1
		  AND (v.escalated_at IS NULL OR v.escalated_at < now() - $2::interval)
		  AND v.created_at < now() - $2::interval
		ORDER BY v.for_date`, maxEscalations, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.DigestEntry{}
	for rows.Next() {
		entry, err := scanDigestEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortByRisk(out)
	return out, nil
}

// RecordEscalation marks a verdict as chased once more.
func (s *Store) RecordEscalation(ctx context.Context, verdictID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE verdicts
		SET escalations = escalations + 1, escalated_at = now()
		WHERE id = $1`, verdictID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
