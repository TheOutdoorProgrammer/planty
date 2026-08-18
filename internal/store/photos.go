package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// SavePhoto records an uploaded image. Bytes live in object storage.
func (s *Store) SavePhoto(ctx context.Context, p plant.Photo) (plant.Photo, error) {
	if p.TakenAt.IsZero() {
		p.TakenAt = time.Now().UTC()
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO photos (plant_id, storage_key, taken_at, caption)
		VALUES ($1, $2, $3, nullif($4,''))
		RETURNING id, plant_id, storage_key, taken_at, coalesce(caption,''),
		          coalesce(vision_findings,''), analyzed_at, created_at`,
		p.PlantID, p.StorageKey, p.TakenAt, p.Caption)

	var out plant.Photo
	err := row.Scan(&out.ID, &out.PlantID, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt)
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
