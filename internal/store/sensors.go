package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
)

const sensorColumns = `
	id, plant_id, coalesce(zone, ''), ha_entity_id, role,
	dry_baseline, wet_baseline, calibrated_at, created_at`

func scanSensor(row pgx.Row) (plant.SensorLink, error) {
	var s plant.SensorLink
	err := row.Scan(&s.ID, &s.PlantID, &s.Zone, &s.HAEntityID, &s.Role,
		&s.DryBaseline, &s.WetBaseline, &s.CalibratedAt, &s.CreatedAt)
	return s, err
}

// LinkSensor attaches a Home Assistant entity to a plant or a zone.
func (s *Store) LinkSensor(ctx context.Context, l plant.SensorLink) (plant.SensorLink, error) {
	if err := l.Valid(); err != nil {
		return plant.SensorLink{}, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO sensor_links (plant_id, zone, ha_entity_id, role)
		VALUES ($1, nullif($2,''), $3, $4)
		ON CONFLICT (ha_entity_id, role) DO UPDATE
			SET plant_id = excluded.plant_id, zone = excluded.zone
		RETURNING `+sensorColumns,
		l.PlantID, l.Zone, l.HAEntityID, l.Role)
	return scanSensor(row)
}

// Calibrate records this probe's own dry and wet baselines.
func (s *Store) Calibrate(ctx context.Context, id uuid.UUID, dry, wet float64) (plant.SensorLink, error) {
	if wet <= dry {
		return plant.SensorLink{}, errors.New("wet baseline must exceed dry baseline")
	}
	row := s.pool.QueryRow(ctx, `
		UPDATE sensor_links
		SET dry_baseline = $2, wet_baseline = $3, calibrated_at = now()
		WHERE id = $1
		RETURNING `+sensorColumns, id, dry, wet)
	l, err := scanSensor(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.SensorLink{}, ErrNotFound
	}
	return l, err
}

// SensorLinks returns every link, optionally narrowed to one plant.
func (s *Store) SensorLinks(ctx context.Context, plantID *uuid.UUID) ([]plant.SensorLink, error) {
	query := `SELECT ` + sensorColumns + ` FROM sensor_links`
	args := []any{}
	if plantID != nil {
		query += ` WHERE plant_id = $1`
		args = append(args, *plantID)
	}
	query += ` ORDER BY created_at`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []plant.SensorLink
	for rows.Next() {
		l, err := scanSensor(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SensorByEntity finds the link for a Home Assistant entity.
func (s *Store) SensorByEntity(ctx context.Context, entityID string, role plant.SensorRole) (plant.SensorLink, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+sensorColumns+`
		FROM sensor_links WHERE ha_entity_id = $1 AND role = $2`, entityID, role)
	l, err := scanSensor(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.SensorLink{}, ErrNotFound
	}
	return l, err
}

// RecordReading stores one sample.
func (s *Store) RecordReading(ctx context.Context, r plant.Reading) error {
	if r.TakenAt.IsZero() {
		r.TakenAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO readings (sensor_link_id, value, unit, taken_at)
		VALUES ($1, $2, nullif($3,''), $4)`,
		r.SensorLinkID, r.Value, r.Unit, r.TakenAt)
	return err
}

// LatestReading returns the most recent sample for a link.
func (s *Store) LatestReading(ctx context.Context, linkID uuid.UUID) (plant.Reading, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, sensor_link_id, value, coalesce(unit,''), taken_at
		FROM readings WHERE sensor_link_id = $1
		ORDER BY taken_at DESC LIMIT 1`, linkID)

	var r plant.Reading
	err := row.Scan(&r.ID, &r.SensorLinkID, &r.Value, &r.Unit, &r.TakenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.Reading{}, ErrNotFound
	}
	return r, err
}

// ReadingsSince returns a link's samples in a window, oldest first.
func (s *Store) ReadingsSince(ctx context.Context, linkID uuid.UUID, since time.Time) ([]plant.Reading, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, sensor_link_id, value, coalesce(unit,''), taken_at
		FROM readings WHERE sensor_link_id = $1 AND taken_at >= $2
		ORDER BY taken_at`, linkID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []plant.Reading
	for rows.Next() {
		var r plant.Reading
		if err := rows.Scan(&r.ID, &r.SensorLinkID, &r.Value, &r.Unit, &r.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MoistureRoseAfter reports whether soil moisture actually climbed after a
// claimed watering. A false answer means the water never reached the roots.
func (s *Store) MoistureRoseAfter(ctx context.Context, linkID uuid.UUID, at time.Time, window time.Duration) (bool, error) {
	var before, after *float64

	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT value FROM readings
			 WHERE sensor_link_id = $1 AND taken_at <= $2
			 ORDER BY taken_at DESC LIMIT 1),
			(SELECT max(value) FROM readings
			 WHERE sensor_link_id = $1 AND taken_at > $2 AND taken_at <= $3)`,
		linkID, at, at.Add(window)).Scan(&before, &after)
	if err != nil {
		return false, err
	}
	if before == nil || after == nil {
		return false, ErrNotFound
	}
	return *after > *before, nil
}
