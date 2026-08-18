package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

// AddObservation records something a person, agent or automation did or saw.
func (s *Store) AddObservation(ctx context.Context, o plant.Observation) (plant.Observation, error) {
	if o.OccurredAt.IsZero() {
		o.OccurredAt = time.Now().UTC()
	}
	if err := o.Valid(); err != nil {
		return plant.Observation{}, err
	}

	row := s.pool.QueryRow(ctx, `
		INSERT INTO observations (plant_id, kind, body, occurred_at, source, actor)
		VALUES ($1, $2, $3, $4, $5, nullif($6,''))
		RETURNING id, plant_id, kind, body, occurred_at, source, coalesce(actor,''), created_at`,
		o.PlantID, o.Kind, o.Body, o.OccurredAt, o.Source, o.Actor)

	var out plant.Observation
	err := row.Scan(&out.ID, &out.PlantID, &out.Kind, &out.Body,
		&out.OccurredAt, &out.Source, &out.Actor, &out.CreatedAt)
	return out, err
}

// Observations returns a plant's history, newest first.
func (s *Store) Observations(ctx context.Context, plantID uuid.UUID, limit int) ([]plant.Observation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, plant_id, kind, body, occurred_at, source, coalesce(actor,''), created_at
		FROM observations
		WHERE plant_id = $1
		ORDER BY occurred_at DESC
		LIMIT $2`, plantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []plant.Observation
	for rows.Next() {
		var o plant.Observation
		if err := rows.Scan(&o.ID, &o.PlantID, &o.Kind, &o.Body,
			&o.OccurredAt, &o.Source, &o.Actor, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// LastWatered returns when a plant was last watered by any means.
func (s *Store) LastWatered(ctx context.Context, plantID uuid.UUID) (time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT occurred_at FROM observations
		WHERE plant_id = $1 AND kind = 'watered'
		ORDER BY occurred_at DESC LIMIT 1`, plantID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrNotFound
	}
	return at, err
}

// AddHarvest records a yield.
func (s *Store) AddHarvest(ctx context.Context, h plant.Harvest) (plant.Harvest, error) {
	if h.OccurredAt.IsZero() {
		h.OccurredAt = time.Now().UTC()
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO harvests (plant_id, occurred_at, quantity, unit, notes)
		VALUES ($1, $2, $3, $4, nullif($5,''))
		RETURNING id, plant_id, occurred_at, quantity, unit, coalesce(notes,''), created_at`,
		h.PlantID, h.OccurredAt, h.Quantity, h.Unit, h.Notes)

	var out plant.Harvest
	err := row.Scan(&out.ID, &out.PlantID, &out.OccurredAt, &out.Quantity,
		&out.Unit, &out.Notes, &out.CreatedAt)
	return out, err
}
