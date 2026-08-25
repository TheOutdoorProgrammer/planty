package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type WateringAttemptOutcome string

const (
	WateringPending          WateringAttemptOutcome = "pending"
	WateringAwaitingEvidence WateringAttemptOutcome = "awaiting_evidence"
	WateringVerified         WateringAttemptOutcome = "verified"
	WateringClogged          WateringAttemptOutcome = "clogged"
	WateringSensorUnknown    WateringAttemptOutcome = "sensor_unknown"
	WateringPumpFailed       WateringAttemptOutcome = "pump_failed"
	WateringMixed            WateringAttemptOutcome = "mixed"
)

type WateringPlantOutcome string

const (
	WateringPlantPending       WateringPlantOutcome = "pending"
	WateringPlantVerified      WateringPlantOutcome = "verified"
	WateringPlantClogged       WateringPlantOutcome = "clogged"
	WateringPlantSensorUnknown WateringPlantOutcome = "sensor_unknown"
	WateringPlantPumpFailed    WateringPlantOutcome = "pump_failed"
)

type PumpActivity string

const (
	PumpActivityUnknown   PumpActivity = "unknown"
	PumpActivityConfirmed PumpActivity = "confirmed"
	PumpActivityInactive  PumpActivity = "inactive"
)

type WateringAttempt struct {
	ID               uuid.UUID
	RequestedAt      time.Time
	PumpStartedAt    *time.Time
	PumpStoppedAt    *time.Time
	PumpSwitch       string
	PumpSensor       string
	RequestedSeconds int
	PumpActivity     PumpActivity
	Outcome          WateringAttemptOutcome
	StartError       string
	StopError        string
	Plants           []plant.Plant
	Results          []WateringPlantEvidence
}

type WateringPlantEvidence struct {
	PlantID   uuid.UUID
	PlantName string
	PlantSlug string
	Outcome   WateringPlantOutcome
	Details   map[string]any
}

func (s *Store) CreateWateringAttempt(ctx context.Context, pumpSwitch, pumpSensor string, runFor time.Duration, plants []plant.Plant) (WateringAttempt, error) {
	seconds := int(runFor / time.Second)
	if runFor > 0 && seconds == 0 {
		seconds = 1
	}
	if pumpSwitch == "" || runFor <= 0 || len(plants) == 0 {
		return WateringAttempt{}, fmt.Errorf("watering attempt needs a pump, positive duration, and plants")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return WateringAttempt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	attempt := WateringAttempt{
		PumpSwitch: pumpSwitch, PumpSensor: pumpSensor, RequestedSeconds: seconds,
		PumpActivity: PumpActivityUnknown, Outcome: WateringPending, Plants: plants,
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO watering_attempts (pump_switch, pump_sensor, requested_seconds)
		VALUES ($1, $2, $3)
		RETURNING id, requested_at`, pumpSwitch, pumpSensor, seconds).
		Scan(&attempt.ID, &attempt.RequestedAt); err != nil {
		return WateringAttempt{}, err
	}
	for _, p := range plants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO watering_attempt_plants (attempt_id, plant_id)
			VALUES ($1, $2)`, attempt.ID, p.ID); err != nil {
			return WateringAttempt{}, err
		}
	}
	return attempt, tx.Commit(ctx)
}

func (s *Store) MarkWateringStarted(ctx context.Context, id uuid.UUID, at time.Time, activity PumpActivity) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE watering_attempts
		SET pump_started_at = $2, pump_activity = $3
		WHERE id = $1 AND outcome = 'pending'`, id, at, activity)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FailWateringStart(ctx context.Context, id uuid.UUID, cause error) error {
	message := "pump start failed"
	if cause != nil {
		message = cause.Error()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE watering_attempts
		SET outcome = 'pump_failed', start_error = $2, finalized_at = now()
		WHERE id = $1 AND outcome = 'pending'`, id, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = tx.Exec(ctx, `
		UPDATE watering_attempt_plants SET outcome = 'pump_failed'
		WHERE attempt_id = $1`, id)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkWateringStopped(ctx context.Context, id uuid.UUID, at time.Time, stopErr error) error {
	message := ""
	if stopErr != nil {
		message = stopErr.Error()
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE watering_attempts
		SET pump_stopped_at = $2, stop_error = $3, outcome = 'awaiting_evidence'
		WHERE id = $1 AND outcome = 'pending' AND pump_started_at IS NOT NULL`, id, at, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) FailWateringStop(ctx context.Context, id uuid.UUID, at time.Time, cause error) error {
	message := "pump stop failed"
	if cause != nil {
		message = cause.Error()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE watering_attempts
		SET pump_stopped_at = $2, stop_error = $3, outcome = 'pump_failed', finalized_at = now()
		WHERE id = $1 AND outcome = 'pending' AND pump_started_at IS NOT NULL`, id, at, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE watering_attempt_plants SET outcome = 'pump_failed'
		WHERE attempt_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) WateringAttemptsReadyForEvidence(ctx context.Context, before time.Time) ([]WateringAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, requested_at, pump_started_at, pump_stopped_at, pump_switch,
		       pump_sensor, requested_seconds, pump_activity, outcome, start_error, stop_error
		FROM watering_attempts
		WHERE outcome = 'awaiting_evidence' AND pump_started_at <= $1
		ORDER BY pump_started_at`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []WateringAttempt
	for rows.Next() {
		var attempt WateringAttempt
		if err := rows.Scan(&attempt.ID, &attempt.RequestedAt, &attempt.PumpStartedAt,
			&attempt.PumpStoppedAt, &attempt.PumpSwitch, &attempt.PumpSensor,
			&attempt.RequestedSeconds, &attempt.PumpActivity, &attempt.Outcome,
			&attempt.StartError, &attempt.StopError); err != nil {
			return nil, err
		}
		plants, err := s.wateringAttemptPlants(ctx, attempt.ID)
		if err != nil {
			return nil, err
		}
		attempt.Plants = plants
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Store) WateringAlertsPending(ctx context.Context) ([]WateringAttempt, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, requested_at, pump_started_at, pump_stopped_at, pump_switch,
		       pump_sensor, requested_seconds, pump_activity, outcome, start_error, stop_error
		FROM watering_attempts
		WHERE outcome IN ('clogged', 'sensor_unknown', 'pump_failed', 'mixed')
		  AND alert_sent_at IS NULL
		ORDER BY finalized_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []WateringAttempt
	for rows.Next() {
		var attempt WateringAttempt
		if err := rows.Scan(&attempt.ID, &attempt.RequestedAt, &attempt.PumpStartedAt,
			&attempt.PumpStoppedAt, &attempt.PumpSwitch, &attempt.PumpSensor,
			&attempt.RequestedSeconds, &attempt.PumpActivity, &attempt.Outcome,
			&attempt.StartError, &attempt.StopError); err != nil {
			return nil, err
		}
		results, err := s.wateringAttemptResults(ctx, attempt.ID)
		if err != nil {
			return nil, err
		}
		attempt.Results = results
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (s *Store) MarkWateringAlert(ctx context.Context, id uuid.UUID, sent bool, alertErr error) error {
	message := ""
	if alertErr != nil {
		message = alertErr.Error()
	}
	var sentAt *time.Time
	if sent {
		now := time.Now().UTC()
		sentAt = &now
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE watering_attempts SET alert_sent_at = $2, alert_error = $3
		WHERE id = $1`, id, sentAt, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) wateringAttemptPlants(ctx context.Context, id uuid.UUID) ([]plant.Plant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumnsFor("p")+`
		FROM watering_attempt_plants wap
		JOIN plants p ON p.id = wap.plant_id
		WHERE wap.attempt_id = $1
		ORDER BY p.common_name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []plant.Plant
	for rows.Next() {
		p, err := scanPlant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) wateringAttemptResults(ctx context.Context, id uuid.UUID) ([]WateringPlantEvidence, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.common_name, p.slug, wap.outcome, wap.evidence
		FROM watering_attempt_plants wap
		JOIN plants p ON p.id = wap.plant_id
		WHERE wap.attempt_id = $1
		ORDER BY p.common_name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WateringPlantEvidence
	for rows.Next() {
		var result WateringPlantEvidence
		var raw []byte
		if err := rows.Scan(&result.PlantID, &result.PlantName, &result.PlantSlug, &result.Outcome, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &result.Details); err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, rows.Err()
}

func (s *Store) FinalizeWateringAttempt(ctx context.Context, id uuid.UUID, outcome WateringAttemptOutcome, evidence []WateringPlantEvidence) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var started time.Time
	if err := tx.QueryRow(ctx, `
		SELECT pump_started_at FROM watering_attempts
		WHERE id = $1 AND outcome = 'awaiting_evidence' FOR UPDATE`, id).Scan(&started); err != nil {
		return classify(err)
	}
	for _, result := range evidence {
		raw, err := json.Marshal(result.Details)
		if err != nil {
			return err
		}
		var observationID *uuid.UUID
		if result.Outcome == WateringPlantVerified {
			var created uuid.UUID
			if err := tx.QueryRow(ctx, `
				INSERT INTO observations (plant_id, kind, body, occurred_at, source, actor)
				VALUES ($1, 'watered', 'LetPot delivery verified by moisture rise', $2, 'automation', 'planty')
				RETURNING id`, result.PlantID, started).Scan(&created); err != nil {
				return err
			}
			observationID = &created
		}
		tag, err := tx.Exec(ctx, `
			UPDATE watering_attempt_plants
			SET outcome = $3, evidence = $4, observation_id = $5
			WHERE attempt_id = $1 AND plant_id = $2`,
			id, result.PlantID, result.Outcome, raw, observationID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE watering_attempts SET outcome = $2, finalized_at = now()
		WHERE id = $1 AND outcome = 'awaiting_evidence'`, id, outcome)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return tx.Commit(ctx)
}
