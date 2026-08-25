package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// HistoryCursor is an opaque API cursor once encoded. The UUID breaks ties when
// two events share the same timestamp so pagination can neither skip nor repeat
// a row at a page boundary.
type HistoryCursor struct {
	At time.Time
	ID uuid.UUID
}

// ObservationsPage returns newest first and a cursor only when older rows exist.
func (s *Store) ObservationsPage(ctx context.Context, plantID uuid.UUID, before *HistoryCursor, limit int) ([]plant.Observation, *HistoryCursor, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var at any
	var id uuid.UUID
	if before != nil {
		at, id = before.At, before.ID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, kind, body, occurred_at, source, coalesce(actor,''), created_at
		FROM observations
		WHERE plant_id = $1
		  AND ($2::timestamptz IS NULL OR (occurred_at, id) < ($2, $3::uuid))
		ORDER BY occurred_at DESC, id DESC
		LIMIT $4`, plantID, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]plant.Observation, 0, limit+1)
	for rows.Next() {
		var o plant.Observation
		if err := rows.Scan(&o.ID, &o.PlantID, &o.Kind, &o.Body,
			&o.OccurredAt, &o.Source, &o.Actor, &o.CreatedAt); err != nil {
			return nil, nil, err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(out) <= limit {
		return out, nil, nil
	}
	out = out[:limit]
	last := out[len(out)-1]
	return out, &HistoryCursor{At: last.OccurredAt, ID: last.ID}, nil
}

// PhotosPage returns the latest page in chronological order, preserving the
// timeline endpoint's existing wire order while still walking backward.
func (s *Store) PhotosPage(ctx context.Context, plantID uuid.UUID, before *HistoryCursor, limit int) ([]plant.Photo, *HistoryCursor, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	var at any
	var id uuid.UUID
	if before != nil {
		at, id = before.At, before.ID
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM photos
		WHERE plant_id = $1
		  AND deletion_requested_at IS NULL
		  AND ($2::timestamptz IS NULL OR (taken_at, id) < ($2, $3::uuid))
		ORDER BY taken_at DESC, id DESC
		LIMIT $4`, plantID, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]plant.Photo, 0, limit+1)
	for rows.Next() {
		var p plant.Photo
		if err := rows.Scan(&p.ID, &p.PlantID, &p.StorageKey, &p.TakenAt,
			&p.Caption, &p.VisionFindings, &p.AnalyzedAt, &p.CreatedAt); err != nil {
			return nil, nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *HistoryCursor
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = &HistoryCursor{At: last.TakenAt, ID: last.ID}
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out, next, nil
}
