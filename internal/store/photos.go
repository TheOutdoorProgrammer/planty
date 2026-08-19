package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// SavePhoto records an uploaded image. Bytes live in object storage.
func (s *Store) SavePhoto(ctx context.Context, p plant.Photo) (plant.Photo, error) {
	if p.TakenAt.IsZero() {
		p.TakenAt = time.Now().UTC()
	}
	// A photograph taken to ask about something nobody owns belongs to no
	// plant, and the zero uuid is how that arrives here.
	var owner any
	if p.PlantID != uuid.Nil {
		owner = p.PlantID
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO photos (plant_id, storage_key, taken_at, caption)
		VALUES ($1, $2, $3, nullif($4,''))
		RETURNING id, plant_id, storage_key, taken_at, coalesce(caption,''),
		          coalesce(vision_findings,''), analyzed_at, created_at`,
		owner, p.StorageKey, p.TakenAt, p.Caption)

	var out plant.Photo
	var back *uuid.UUID
	err := row.Scan(&out.ID, &back, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt)
	if back != nil {
		out.PlantID = *back
	}
	return out, err
}

// Photo returns one image by id, whether or not it belongs to a plant.
func (s *Store) Photo(ctx context.Context, id uuid.UUID) (plant.Photo, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM photos WHERE id = $1`, id)

	var out plant.Photo
	var owner *uuid.UUID
	err := row.Scan(&out.ID, &owner, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Photo{}, ErrNotFound
	}
	if owner != nil {
		out.PlantID = *owner
	}
	return out, err
}

// Photos returns a plant's timeline, oldest first so a model reading them in
// order sees the change rather than having to reconstruct it.
func (s *Store) Photos(ctx context.Context, plantID uuid.UUID, limit int) ([]plant.Photo, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM (
			SELECT * FROM photos WHERE plant_id = $1
			ORDER BY taken_at DESC LIMIT $2
		) recent
		ORDER BY taken_at`, plantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.Photo{}
	for rows.Next() {
		var p plant.Photo
		if err := rows.Scan(&p.ID, &p.PlantID, &p.StorageKey, &p.TakenAt,
			&p.Caption, &p.VisionFindings, &p.AnalyzedAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// NewestPhotos returns the most recent photograph of each of the given plants,
// keyed by plant. One query rather than one per plant: the library screen shows
// every plant at once, and a request per row is a list that loads in steps.
func (s *Store) NewestPhotos(ctx context.Context, plantIDs []uuid.UUID) (map[uuid.UUID]plant.Photo, error) {
	newest := make(map[uuid.UUID]plant.Photo, len(plantIDs))
	if len(plantIDs) == 0 {
		return newest, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (plant_id)
		       id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM photos
		WHERE plant_id = ANY($1)
		ORDER BY plant_id, taken_at DESC`, plantIDs)
	if err != nil {
		return nil, fmt.Errorf("newest photos: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p plant.Photo
		if err := rows.Scan(&p.ID, &p.PlantID, &p.StorageKey, &p.TakenAt,
			&p.Caption, &p.VisionFindings, &p.AnalyzedAt, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan newest photo: %w", err)
		}
		newest[p.PlantID] = p
	}
	return newest, rows.Err()
}

// RecordVision stores what the model saw, kept apart from human captions so a
// wrong machine reading is never mistaken later for a first-hand observation.
func (s *Store) RecordVision(ctx context.Context, photoID uuid.UUID, findings string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE photos SET vision_findings = $2, analyzed_at = now() WHERE id = $1`,
		photoID, findings)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
