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
	saved, _, err := s.SavePhotoOnce(ctx, p)
	return saved, err
}

// SavePhotoOnce atomically claims a content hash and reports whether this call
// inserted it. Callers use inserted to remove a concurrently uploaded losing
// object instead of leaking bytes that no database row references.
func (s *Store) SavePhotoOnce(ctx context.Context, p plant.Photo) (plant.Photo, bool, error) {
	if p.TakenAt.IsZero() {
		p.TakenAt = time.Now().UTC()
	}
	// A photograph taken to ask about something nobody owns belongs to no
	// plant, and the zero uuid is how that arrives here.
	var owner any
	if p.PlantID != uuid.Nil {
		owner = p.PlantID
	}

	conflict := ""
	if p.ContentHash != "" {
		if p.PlantID == uuid.Nil {
			conflict = ` ON CONFLICT (content_hash)
				WHERE content_hash IS NOT NULL AND plant_id IS NULL
				  AND deletion_requested_at IS NULL DO NOTHING`
		} else {
			conflict = ` ON CONFLICT (plant_id, content_hash)
				WHERE content_hash IS NOT NULL AND plant_id IS NOT NULL
				  AND deletion_requested_at IS NULL DO NOTHING`
		}
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO photos (plant_id, storage_key, taken_at, caption, content_hash)
		VALUES ($1, $2, $3, nullif($4,''), nullif($5,''))
		`+conflict+`
		RETURNING id, plant_id, storage_key, taken_at, coalesce(caption,''),
		          coalesce(vision_findings,''), analyzed_at, created_at`,
		owner, p.StorageKey, p.TakenAt, p.Caption, p.ContentHash)

	var out plant.Photo
	var back *uuid.UUID
	err := row.Scan(&out.ID, &back, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) && p.ContentHash != "" {
		existing, found, lookupErr := s.PhotoByHash(ctx, p.PlantID, p.ContentHash)
		if lookupErr != nil {
			return plant.Photo{}, false, lookupErr
		}
		if !found {
			return plant.Photo{}, false, fmt.Errorf("photo hash conflict disappeared")
		}
		return existing, false, nil
	}
	if err != nil {
		return plant.Photo{}, false, err
	}
	if back != nil {
		out.PlantID = *back
	}
	out.ContentHash = p.ContentHash
	return out, true, nil
}

// PhotoByHash finds an identical upload already filed against the same owner,
// so one capture that arrives twice stays one photograph.
func (s *Store) PhotoByHash(ctx context.Context, plantID uuid.UUID, hash string) (plant.Photo, bool, error) {
	if hash == "" {
		return plant.Photo{}, false, nil
	}

	var owner any
	if plantID != uuid.Nil {
		owner = plantID
	}
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM photos
		WHERE content_hash = $1 AND plant_id IS NOT DISTINCT FROM $2
		  AND deletion_requested_at IS NULL`,
		hash, owner)

	var out plant.Photo
	var back *uuid.UUID
	err := row.Scan(&out.ID, &back, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Photo{}, false, nil
	}
	if err != nil {
		return plant.Photo{}, false, err
	}
	if back != nil {
		out.PlantID = *back
	}
	out.ContentHash = hash
	return out, true, nil
}

// AttachPhoto gives an unowned photograph to a plant. Refuses to steal one
// already filed elsewhere, since a picture moving plants silently is how a
// timeline stops being evidence of anything.
func (s *Store) AttachPhoto(ctx context.Context, photoID, plantID uuid.UUID, caption string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE photos SET plant_id = $2, caption = coalesce(nullif($3,''), caption)
		WHERE id = $1 AND (plant_id IS NULL OR plant_id = $2)`, photoID, plantID, caption)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: no such photograph, or it already belongs to another plant", plant.ErrInvalid)
	}
	return nil
}

// Photo returns one image by id, whether or not it belongs to a plant.
func (s *Store) Photo(ctx context.Context, id uuid.UUID) (plant.Photo, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, storage_key, taken_at, coalesce(caption,''),
		       coalesce(vision_findings,''), analyzed_at, created_at
		FROM photos WHERE id = $1 AND deletion_requested_at IS NULL`, id)

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
			SELECT * FROM photos WHERE plant_id = $1 AND deletion_requested_at IS NULL
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
		WHERE plant_id = ANY($1) AND deletion_requested_at IS NULL
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

// RequestPhotoDeletion hides a photograph and makes object cleanup durable.
// The row remains until its bytes are gone, so a failed object call can retry.
func (s *Store) RequestPhotoDeletion(ctx context.Context, id uuid.UUID) (plant.Photo, error) {
	row := s.pool.QueryRow(ctx, `
		UPDATE photos SET deletion_requested_at = coalesce(deletion_requested_at, now())
		WHERE id = $1
		RETURNING id, plant_id, storage_key, taken_at, coalesce(caption,''),
		          coalesce(vision_findings,''), analyzed_at, created_at`, id)
	var out plant.Photo
	var owner *uuid.UUID
	if err := row.Scan(&out.ID, &owner, &out.StorageKey, &out.TakenAt,
		&out.Caption, &out.VisionFindings, &out.AnalyzedAt, &out.CreatedAt); errors.Is(err, pgx.ErrNoRows) {
		return plant.Photo{}, ErrNotFound
	} else if err != nil {
		return plant.Photo{}, err
	}
	if owner != nil {
		out.PlantID = *owner
	}
	return out, nil
}

// ClaimExpiredScratchPhotos marks old unowned conversation images before
// returning them, keeping them hidden while an object deletion is retried.
func (s *Store) ClaimExpiredScratchPhotos(ctx context.Context, before time.Time, limit int) ([]plant.Photo, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		WITH claimed AS (
			SELECT id FROM photos
			WHERE (deletion_requested_at IS NOT NULL OR (plant_id IS NULL AND created_at < $1))
			ORDER BY coalesce(deletion_requested_at, created_at), id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE photos p
		SET deletion_requested_at = coalesce(p.deletion_requested_at, now())
		FROM claimed
		WHERE p.id = claimed.id
		RETURNING p.id, p.plant_id, p.storage_key, p.taken_at, coalesce(p.caption,''),
		          coalesce(p.vision_findings,''), p.analyzed_at, p.created_at`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plant.Photo
	for rows.Next() {
		var photo plant.Photo
		var owner *uuid.UUID
		if err := rows.Scan(&photo.ID, &owner, &photo.StorageKey, &photo.TakenAt,
			&photo.Caption, &photo.VisionFindings, &photo.AnalyzedAt, &photo.CreatedAt); err != nil {
			return nil, err
		}
		if owner != nil {
			photo.PlantID = *owner
		}
		out = append(out, photo)
	}
	return out, rows.Err()
}

func (s *Store) FinalizePhotoDeletion(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM photos WHERE id = $1 AND deletion_requested_at IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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
