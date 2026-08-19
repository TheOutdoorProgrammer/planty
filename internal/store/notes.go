package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

const noteColumns = `id, plant_id, coalesce(title,''), body, created_at, updated_at`

func scanNote(row pgx.Row) (plant.Note, error) {
	var n plant.Note
	var owner *uuid.UUID
	err := row.Scan(&n.ID, &owner, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Note{}, ErrNotFound
	}
	// Null means the household rather than any plant.
	if owner != nil {
		n.PlantID = *owner
	}
	return n, err
}

// Notes returns notes newest first: a plant's own, or the household's when
// the zero uuid is given. The household's are about the place rather than
// anything growing in it, and are read on every consultation.
func (s *Store) Notes(ctx context.Context, plantID uuid.UUID) ([]plant.Note, error) {
	var owner any
	if plantID != uuid.Nil {
		owner = plantID
	}
	rows, err := s.pool.Query(ctx, `SELECT `+noteColumns+`
		FROM notes WHERE plant_id IS NOT DISTINCT FROM $1
		ORDER BY created_at DESC`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []plant.Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Note returns one note by id.
func (s *Store) Note(ctx context.Context, id uuid.UUID) (plant.Note, error) {
	return scanNote(s.pool.QueryRow(ctx,
		`SELECT `+noteColumns+` FROM notes WHERE id = $1`, id))
}

// AddNote writes a new note against a plant.
func (s *Store) AddNote(ctx context.Context, n plant.Note) (plant.Note, error) {
	if strings.TrimSpace(n.Body) == "" {
		return plant.Note{}, invalidNote("a note needs something in it")
	}
	var owner any
	if n.PlantID != uuid.Nil {
		owner = n.PlantID
	}
	return scanNote(s.pool.QueryRow(ctx, `
		INSERT INTO notes (plant_id, title, body)
		VALUES ($1, nullif($2,''), $3)
		RETURNING `+noteColumns, owner, n.Title, strings.TrimSpace(n.Body)))
}

// UpdateNote changes a note's title, body, or both. A nil field is left alone,
// so editing the body of a titled note does not silently drop its title.
func (s *Store) UpdateNote(ctx context.Context, id uuid.UUID, title, body *string) (plant.Note, error) {
	if title == nil && body == nil {
		return plant.Note{}, invalidNote("nothing to change")
	}
	if body != nil && strings.TrimSpace(*body) == "" {
		return plant.Note{}, invalidNote("a note needs something in it; delete it instead")
	}
	return scanNote(s.pool.QueryRow(ctx, `
		UPDATE notes
		SET title = coalesce(nullif($2,''), title),
		    body  = coalesce($3, body),
		    updated_at = now()
		WHERE id = $1
		RETURNING `+noteColumns, id, derefOr(title, ""), body))
}

// DeleteNote removes one note, reporting whether there was one to remove.
func (s *Store) DeleteNote(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func derefOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return *s
}

func invalidNote(reason string) error {
	return errors.Join(plant.ErrInvalid, errors.New(reason))
}
