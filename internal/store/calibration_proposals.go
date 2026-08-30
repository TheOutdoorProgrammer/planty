package store

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const CalibrationProposalCooldown = 72 * time.Hour

const calibrationProposalColumns = `id, sensor_link_id, plant_id, reading_id,
	actual_value, unit, current_dry, current_wet, proposed_dry, proposed_wet,
	current_relative, proposed_relative, reason, model_version, status,
	created_at, resolved_at, resolved_by`

func scanCalibrationProposal(row pgx.Row) (plant.CalibrationProposal, error) {
	var proposal plant.CalibrationProposal
	err := row.Scan(
		&proposal.ID, &proposal.SensorLinkID, &proposal.PlantID, &proposal.ReadingID,
		&proposal.ActualValue, &proposal.Unit, &proposal.CurrentDry, &proposal.CurrentWet,
		&proposal.ProposedDry, &proposal.ProposedWet, &proposal.CurrentRelative,
		&proposal.ProposedRelative, &proposal.Reason, &proposal.ModelVersion,
		&proposal.Status, &proposal.CreatedAt, &proposal.ResolvedAt, &proposal.ResolvedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.CalibrationProposal{}, ErrNotFound
	}
	return proposal, classify(err)
}

func (s *Store) ProposeCalibration(ctx context.Context, proposal plant.CalibrationProposal) (plant.CalibrationProposal, bool, error) {
	if proposal.SensorLinkID == uuid.Nil || proposal.ReadingID == uuid.Nil || strings.TrimSpace(proposal.Reason) == "" {
		return plant.CalibrationProposal{}, false, fmt.Errorf("%w: sensor, reading, and reason are required", plant.ErrInvalid)
	}
	if proposal.ProposedWet <= proposal.ProposedDry {
		return plant.CalibrationProposal{}, false, fmt.Errorf("%w: proposed wet baseline must exceed dry baseline", plant.ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.CalibrationProposal{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var link plant.SensorLink
	if err := tx.QueryRow(ctx, `SELECT `+sensorColumns+` FROM sensor_links
		WHERE id = $1 FOR UPDATE`, proposal.SensorLinkID).Scan(
		&link.ID, &link.PlantID, &link.Zone, &link.HAEntityID, &link.Role,
		&link.DryBaseline, &link.WetBaseline, &link.CalibratedAt, &link.CreatedAt,
	); err != nil {
		return plant.CalibrationProposal{}, false, classify(err)
	}
	if !link.Calibrated() || link.PlantID == nil {
		return plant.CalibrationProposal{}, false, fmt.Errorf("%w: proposals require a calibrated plant soil sensor", plant.ErrInvalid)
	}
	var reading plant.Reading
	if err := tx.QueryRow(ctx, `SELECT id, sensor_link_id, value, coalesce(unit,''), taken_at
		FROM readings WHERE id = $1 AND sensor_link_id = $2`, proposal.ReadingID, proposal.SensorLinkID).Scan(
		&reading.ID, &reading.SensorLinkID, &reading.Value, &reading.Unit, &reading.TakenAt,
	); err != nil {
		return plant.CalibrationProposal{}, false, classify(err)
	}
	if time.Since(reading.TakenAt) > plant.StaleAfter {
		return plant.CalibrationProposal{}, false, fmt.Errorf("%w: calibration proposal evidence is stale", plant.ErrInvalid)
	}
	var recent bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM sensor_calibration_proposals
		WHERE sensor_link_id = $1 AND (status = 'pending' OR created_at > now() - ($2 * interval '1 second'))
	)`, proposal.SensorLinkID, int(CalibrationProposalCooldown/time.Second)).Scan(&recent); err != nil {
		return plant.CalibrationProposal{}, false, err
	}
	if recent {
		return plant.CalibrationProposal{}, false, tx.Commit(ctx)
	}

	span := *link.WetBaseline - *link.DryBaseline
	minimumChange := math.Max(1, span*0.05)
	if math.Abs(proposal.ProposedDry-*link.DryBaseline) < minimumChange &&
		math.Abs(proposal.ProposedWet-*link.WetBaseline) < minimumChange {
		return plant.CalibrationProposal{}, false, fmt.Errorf("%w: proposed calibration change is too small", plant.ErrInvalid)
	}
	proposal.PlantID = *link.PlantID
	proposal.ActualValue = reading.Value
	proposal.Unit = reading.Unit
	proposal.CurrentDry = *link.DryBaseline
	proposal.CurrentWet = *link.WetBaseline
	proposal.CurrentRelative = relativeValue(reading.Value, proposal.CurrentDry, proposal.CurrentWet)
	proposal.ProposedRelative = relativeValue(reading.Value, proposal.ProposedDry, proposal.ProposedWet)
	proposal.Status = plant.CalibrationPending

	created, err := scanCalibrationProposal(tx.QueryRow(ctx, `INSERT INTO sensor_calibration_proposals
		(sensor_link_id, plant_id, reading_id, actual_value, unit, current_dry, current_wet,
		 proposed_dry, proposed_wet, current_relative, proposed_relative, reason, model_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING `+calibrationProposalColumns,
		proposal.SensorLinkID, proposal.PlantID, proposal.ReadingID, proposal.ActualValue,
		proposal.Unit, proposal.CurrentDry, proposal.CurrentWet, proposal.ProposedDry,
		proposal.ProposedWet, proposal.CurrentRelative, proposal.ProposedRelative,
		strings.TrimSpace(proposal.Reason), proposal.ModelVersion,
	))
	if err != nil {
		return plant.CalibrationProposal{}, false, err
	}
	return created, true, tx.Commit(ctx)
}

func relativeValue(raw, dry, wet float64) float64 {
	return min(max((raw-dry)/(wet-dry), 0), 1)
}

func (s *Store) PendingCalibrationProposals(ctx context.Context, plantID uuid.UUID) ([]plant.CalibrationProposal, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+calibrationProposalColumns+`
		FROM sensor_calibration_proposals WHERE plant_id = $1 AND status = 'pending'
		ORDER BY created_at DESC`, plantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plant.CalibrationProposal{}
	for rows.Next() {
		proposal, err := scanCalibrationProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, proposal)
	}
	return out, rows.Err()
}

func (s *Store) ResolveCalibrationProposal(ctx context.Context, id uuid.UUID, approve bool, actor string) (plant.CalibrationProposal, error) {
	if id == uuid.Nil || strings.TrimSpace(actor) == "" {
		return plant.CalibrationProposal{}, fmt.Errorf("%w: proposal id and actor are required", plant.ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.CalibrationProposal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	proposal, err := scanCalibrationProposal(tx.QueryRow(ctx, `SELECT `+calibrationProposalColumns+`
		FROM sensor_calibration_proposals WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, ErrNotFound) {
		return plant.CalibrationProposal{}, ErrNotFound
	}
	if err != nil {
		return plant.CalibrationProposal{}, err
	}
	if proposal.Status != plant.CalibrationPending {
		return plant.CalibrationProposal{}, fmt.Errorf("%w: calibration proposal is already resolved", plant.ErrInvalid)
	}
	status := plant.CalibrationDenied
	if approve {
		status = plant.CalibrationApproved
		command, err := tx.Exec(ctx, `UPDATE sensor_links SET dry_baseline = $2,
			wet_baseline = $3, calibrated_at = now()
			WHERE id = $1 AND dry_baseline = $4 AND wet_baseline = $5`,
			proposal.SensorLinkID, proposal.ProposedDry, proposal.ProposedWet,
			proposal.CurrentDry, proposal.CurrentWet)
		if err != nil {
			return plant.CalibrationProposal{}, err
		}
		if command.RowsAffected() != 1 {
			return plant.CalibrationProposal{}, fmt.Errorf("%w: sensor calibration changed after this proposal", plant.ErrInvalid)
		}
	}
	return scanCalibrationProposalAfterCommit(ctx, tx, id, status, strings.TrimSpace(actor))
}

func scanCalibrationProposalAfterCommit(ctx context.Context, tx pgx.Tx, id uuid.UUID, status plant.CalibrationProposalStatus, actor string) (plant.CalibrationProposal, error) {
	resolved, err := scanCalibrationProposal(tx.QueryRow(ctx, `UPDATE sensor_calibration_proposals
		SET status = $2, resolved_at = now(), resolved_by = $3 WHERE id = $1
		RETURNING `+calibrationProposalColumns, id, status, actor))
	if err != nil {
		return plant.CalibrationProposal{}, err
	}
	return resolved, tx.Commit(ctx)
}
