package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func (s *Store) DetectWatering(ctx context.Context, now time.Time) (detected int, err error) {
	ctx, span := otel.Tracer("planty/store").Start(ctx, "planty.watering.detect")
	defer func() {
		span.SetAttributes(attribute.Int("watering.detected", detected))
		if err != nil {
			span.SetStatus(codes.Error, "watering detection failed")
		}
		span.End()
	}()
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM plants p
		JOIN verdicts v ON v.plant_id = p.id
		WHERE p.archived_at IS NULL AND p.status IN ('alive', 'struggling')
		AND p.watering_method = 'hand' AND v.action = 'water' AND v.acknowledged_at IS NULL
		ORDER BY p.id`)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		credited, err := s.detectPlantWatering(ctx, id, now)
		if err != nil {
			return detected, err
		}
		if credited {
			detected++
		}
	}
	return detected, nil
}

func (s *Store) detectPlantWatering(ctx context.Context, id uuid.UUID, now time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked uuid.UUID
	// Allow foreign-key checks from concurrent probe calibration while serializing care.
	err = tx.QueryRow(ctx, `SELECT id FROM plants WHERE id = $1
		AND archived_at IS NULL AND status IN ('alive', 'struggling') AND watering_method = 'hand'
		FOR NO KEY UPDATE`, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var verdictID uuid.UUID
	var requestedAt time.Time
	err = tx.QueryRow(ctx, `SELECT id, created_at FROM verdicts
		WHERE plant_id = $1 AND action = 'water' AND acknowledged_at IS NULL FOR UPDATE`, id).
		Scan(&verdictID, &requestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var recentCare bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM observations
		WHERE plant_id = $1 AND kind IN ('watered', 'repotted', 'moved') AND occurred_at >= $2)`,
		id, now.Add(-plant.WateringDetectionWindow)).Scan(&recentCare); err != nil {
		return false, err
	}
	if recentCare {
		return false, nil
	}

	rows, err := tx.Query(ctx, `SELECT `+sensorColumns+` FROM sensor_links
		WHERE plant_id = $1 AND role = 'soil_moisture' AND assigned_at <= $2
		AND calibrated_at <= $3 ORDER BY id FOR UPDATE`, id, now.Add(-24*time.Hour), now.Add(-plant.WateringDetectionWindow))
	if err != nil {
		return false, err
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (plant.SensorLink, error) { return scanSensor(row) })
	if err != nil {
		return false, err
	}
	for _, link := range links {
		rows, err := tx.Query(ctx, `SELECT id, sensor_link_id, value, coalesce(unit,''), taken_at, reported_at
			FROM readings WHERE sensor_link_id = $1 AND taken_at >= $2 AND taken_at <= $3
			ORDER BY taken_at, id`, link.ID, now.Add(-plant.WateringDetectionWindow), now)
		if err != nil {
			return false, err
		}
		readings, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (plant.Reading, error) {
			var r plant.Reading
			err := row.Scan(&r.ID, &r.SensorLinkID, &r.Value, &r.Unit, &r.TakenAt, &r.ReportedAt)
			return r, err
		})
		if err != nil {
			return false, err
		}
		rise := plant.DetectWateringRise(link, readings, now)
		if rise == nil || rise.Rise.ReportedAt.Before(requestedAt) {
			continue
		}
		body := fmt.Sprintf("Watering detected automatically: soil moisture rose from %.2f%s to %.2f%s and remained at %.2f%s.",
			rise.Baseline.Value, rise.Baseline.Unit, rise.Rise.Value, rise.Rise.Unit,
			rise.Confirmation.Value, rise.Confirmation.Unit)
		observation, err := completeVerdictTx(ctx, tx, verdictID, plant.Observation{
			PlantID: id, Kind: plant.ObservedWatered, Body: body,
			OccurredAt: *rise.Rise.ReportedAt, Source: plant.SourceAutomation, Actor: "soil-moisture-detection",
		})
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO watering_detections
			(observation_id, verdict_id, sensor_link_id, baseline_reading_id, rise_reading_id,
			 confirmation_reading_id, dry_baseline, wet_baseline, detected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, observation.ID, verdictID, link.ID,
			rise.Baseline.ID, rise.Rise.ID, rise.Confirmation.ID, *link.DryBaseline, *link.WetBaseline, now); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}
