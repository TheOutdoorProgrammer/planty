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
		WITH RECURSIVE ancestry AS (
			SELECT $1::uuid AS plant_id, 'infinity'::timestamptz AS cutoff, 0 AS depth
			UNION ALL
			SELECT lineage.source_plant_id, least(ancestry.cutoff, lineage.derived_at), ancestry.depth + 1
			FROM ancestry
			JOIN plant_lineage lineage ON lineage.child_plant_id = ancestry.plant_id
		)
		SELECT observation.id, observation.plant_id, observation.kind, observation.body,
		       observation.occurred_at, observation.source, coalesce(observation.actor,''),
		       observation.created_at, source.slug, source.common_name, ancestry.depth
		FROM observations observation
		JOIN ancestry ON ancestry.plant_id = observation.plant_id
		JOIN plants source ON source.id = observation.plant_id
		WHERE observation.occurred_at <= ancestry.cutoff
		  AND ($2::timestamptz IS NULL OR (observation.occurred_at, observation.id) < ($2, $3::uuid))
		ORDER BY observation.occurred_at DESC, observation.id DESC
		LIMIT $4`, plantID, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]plant.Observation, 0, limit+1)
	for rows.Next() {
		var o plant.Observation
		var sourceSlug, sourceName string
		var depth int
		if err := rows.Scan(&o.ID, &o.PlantID, &o.Kind, &o.Body,
			&o.OccurredAt, &o.Source, &o.Actor, &o.CreatedAt,
			&sourceSlug, &sourceName, &depth); err != nil {
			return nil, nil, err
		}
		if depth > 0 {
			o.InheritedFrom = &plant.HistorySource{PlantID: o.PlantID, Slug: sourceSlug, CommonName: sourceName}
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
		WITH RECURSIVE ancestry AS (
			SELECT $1::uuid AS plant_id, 'infinity'::timestamptz AS cutoff, 0 AS depth
			UNION ALL
			SELECT lineage.source_plant_id, least(ancestry.cutoff, lineage.derived_at), ancestry.depth + 1
			FROM ancestry
			JOIN plant_lineage lineage ON lineage.child_plant_id = ancestry.plant_id
		)
		SELECT photo.id, photo.plant_id, photo.storage_key, photo.taken_at, coalesce(photo.caption,''),
		       coalesce(photo.vision_findings,''), photo.analyzed_at, photo.created_at,
		       source.slug, source.common_name, ancestry.depth
		FROM photos photo
		JOIN ancestry ON ancestry.plant_id = photo.plant_id
		JOIN plants source ON source.id = photo.plant_id
		WHERE photo.taken_at <= ancestry.cutoff
		  AND photo.deletion_requested_at IS NULL
		  AND ($2::timestamptz IS NULL OR (photo.taken_at, photo.id) < ($2, $3::uuid))
		ORDER BY photo.taken_at DESC, photo.id DESC
		LIMIT $4`, plantID, at, id, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := make([]plant.Photo, 0, limit+1)
	for rows.Next() {
		var p plant.Photo
		var sourceSlug, sourceName string
		var depth int
		if err := rows.Scan(&p.ID, &p.PlantID, &p.StorageKey, &p.TakenAt,
			&p.Caption, &p.VisionFindings, &p.AnalyzedAt, &p.CreatedAt,
			&sourceSlug, &sourceName, &depth); err != nil {
			return nil, nil, err
		}
		if depth > 0 {
			p.InheritedFrom = &plant.HistorySource{PlantID: p.PlantID, Slug: sourceSlug, CommonName: sourceName}
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
