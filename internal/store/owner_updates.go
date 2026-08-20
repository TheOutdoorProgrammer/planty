package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// ObservationsSince returns the human/automation actions that happened in the
// owner-update window, oldest first so the summary reads as a week rather than
// a pile of latest-first database rows.
func (s *Store) ObservationsSince(ctx context.Context, plantID uuid.UUID, since time.Time) ([]plant.Observation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, kind, coalesce(body,''), occurred_at, source,
		       coalesce(actor,''), created_at
		FROM observations
		WHERE plant_id = $1 AND occurred_at >= $2
		ORDER BY occurred_at`, plantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plant.Observation
	for rows.Next() {
		var item plant.Observation
		if err := rows.Scan(&item.ID, &item.PlantID, &item.Kind, &item.Body,
			&item.OccurredAt, &item.Source, &item.Actor, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// VerdictsSince returns the daily conclusions in the update window. Evidence
// internals are intentionally omitted: an owner wants what changed and what was
// done, not sensor IDs or model debugging metadata.
func (s *Store) VerdictsSince(ctx context.Context, plantID uuid.UUID, since time.Time) ([]plant.Verdict, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, for_date, action, reasoning, confidence,
		       created_at, acknowledged_at, coalesce(escalations, 0)
		FROM verdicts
		WHERE plant_id = $1 AND created_at >= $2
		ORDER BY for_date`, plantID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plant.Verdict
	for rows.Next() {
		var item plant.Verdict
		if err := rows.Scan(&item.ID, &item.PlantID, &item.ForDate, &item.Action,
			&item.Reasoning, &item.Confidence, &item.CreatedAt,
			&item.AcknowledgedAt, &item.Escalations); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
