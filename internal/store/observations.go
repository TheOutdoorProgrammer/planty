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
	return out, classify(err)
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

	out := []plant.Observation{}
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
	return s.LastObserved(ctx, plantID, plant.ObservedWatered)
}

// LastObserved is when something of one kind last happened to a plant, which
// is what every reminder is measured against.
func (s *Store) LastObserved(ctx context.Context, plantID uuid.UUID,
	kind plant.ObservationKind) (time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT occurred_at FROM observations
		WHERE plant_id = $1 AND kind = $2
		ORDER BY occurred_at DESC LIMIT 1`, plantID, kind).Scan(&at)
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
	if err := h.Valid(); err != nil {
		return plant.Harvest{}, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO harvests (plant_id, occurred_at, quantity, unit, notes)
		VALUES ($1, $2, $3, $4, nullif($5,''))
		RETURNING id, plant_id, occurred_at, quantity, unit, coalesce(notes,''), created_at, updated_at`,
		h.PlantID, h.OccurredAt, h.Quantity, h.Unit, h.Notes)

	var out plant.Harvest
	err := row.Scan(&out.ID, &out.PlantID, &out.OccurredAt, &out.Quantity,
		&out.Unit, &out.Notes, &out.CreatedAt, &out.UpdatedAt)
	return out, classify(err)
}

// HarvestRecord carries enough plant identity for a garden-wide history to be
// useful without making one plant lookup per row.
type HarvestRecord struct {
	Slug       string `json:"slug"`
	CommonName string `json:"common_name"`
	plant.Harvest
}

// Harvests returns yield newest first, optionally narrowed to one plant.
func (s *Store) Harvests(ctx context.Context, plantID *uuid.UUID) ([]HarvestRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.slug, p.common_name,
		       h.id, h.plant_id, h.occurred_at, h.quantity, h.unit,
		       coalesce(h.notes, ''), h.created_at, h.updated_at
		FROM harvests h
		JOIN plants p ON p.id = h.plant_id
		WHERE ($1::uuid IS NULL OR h.plant_id = $1)
		ORDER BY h.occurred_at DESC, h.created_at DESC`, plantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []HarvestRecord{}
	for rows.Next() {
		var record HarvestRecord
		if err := rows.Scan(
			&record.Slug, &record.CommonName,
			&record.ID, &record.PlantID, &record.OccurredAt,
			&record.Quantity, &record.Unit, &record.Notes, &record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

func (s *Store) UpdateHarvest(ctx context.Context, h plant.Harvest) (plant.Harvest, error) {
	if err := h.Valid(); err != nil {
		return plant.Harvest{}, err
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE harvests
		SET occurred_at = $3, quantity = $4, unit = $5, notes = nullif($6,''), updated_at = now()
		WHERE id = $1 AND plant_id = $2
		RETURNING id, plant_id, occurred_at, quantity, unit, coalesce(notes,''), created_at, updated_at`,
		h.ID, h.PlantID, h.OccurredAt, h.Quantity, h.Unit, h.Notes)
	var out plant.Harvest
	if err := row.Scan(&out.ID, &out.PlantID, &out.OccurredAt, &out.Quantity,
		&out.Unit, &out.Notes, &out.CreatedAt, &out.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return plant.Harvest{}, ErrNotFound
	} else if err != nil {
		return plant.Harvest{}, classify(err)
	}
	return out, nil
}

func (s *Store) Harvest(ctx context.Context, id uuid.UUID) (plant.Harvest, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, plant_id, occurred_at, quantity, unit, coalesce(notes,''), created_at, updated_at
		FROM harvests WHERE id = $1`, id)
	var out plant.Harvest
	if err := row.Scan(&out.ID, &out.PlantID, &out.OccurredAt, &out.Quantity,
		&out.Unit, &out.Notes, &out.CreatedAt, &out.UpdatedAt); errors.Is(err, pgx.ErrNoRows) {
		return plant.Harvest{}, ErrNotFound
	} else if err != nil {
		return plant.Harvest{}, err
	}
	return out, nil
}

func (s *Store) DeleteHarvest(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM harvests WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type HarvestSummary struct {
	PlantID    uuid.UUID `json:"plant_id"`
	Slug       string    `json:"slug"`
	CommonName string    `json:"common_name"`
	Unit       string    `json:"unit"`
	Season     string    `json:"season"`
	Year       int       `json:"year"`
	Quantity   float64   `json:"quantity"`
	Count      int       `json:"count"`
}

func (s *Store) HarvestSummary(ctx context.Context) ([]HarvestSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.slug, p.common_name, h.unit,
		       CASE
		         WHEN extract(month FROM h.occurred_at) IN (12, 1, 2) THEN 'winter'
		         WHEN extract(month FROM h.occurred_at) IN (3, 4, 5) THEN 'spring'
		         WHEN extract(month FROM h.occurred_at) IN (6, 7, 8) THEN 'summer'
		         ELSE 'fall'
		       END AS season,
		       CASE WHEN extract(month FROM h.occurred_at) = 12
		            THEN extract(year FROM h.occurred_at)::int + 1
		            ELSE extract(year FROM h.occurred_at)::int END AS season_year,
		       sum(h.quantity)::float8, count(*)::int
		FROM harvests h
		JOIN plants p ON p.id = h.plant_id
		GROUP BY p.id, p.slug, p.common_name, h.unit, season, season_year
		ORDER BY season_year DESC, season DESC, p.common_name, h.unit`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HarvestSummary{}
	for rows.Next() {
		var summary HarvestSummary
		if err := rows.Scan(&summary.PlantID, &summary.Slug, &summary.CommonName,
			&summary.Unit, &summary.Season, &summary.Year, &summary.Quantity, &summary.Count); err != nil {
			return nil, err
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}
