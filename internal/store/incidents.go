package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const incidentColumns = `id, status, suspected_factor_type, suspected_factor_ref,
	summary, reason, confidence, evidence, detected_run_id, first_seen_at, last_seen_at,
	acknowledged_at, acknowledged_by, resolved_at, resolved_by, resolution,
	conclusion, created_at, updated_at`

type IncidentSignal struct {
	Plant      plant.Plant
	VerdictID  uuid.UUID
	Action     plant.Action
	Reasoning  string
	Confidence float64
}

type IncidentCareSignal struct {
	ID         uuid.UUID
	PlantID    uuid.UUID
	Kind       plant.ObservationKind
	OccurredAt time.Time
}

type IncidentEnvironmentFailure struct {
	SensorLinkID uuid.UUID
	Area         string
	EntityID     string
}

type IncidentActuatorFailure struct {
	EventID    uuid.UUID
	ActuatorID uuid.UUID
	PlantID    uuid.UUID
}

// IncidentSignalsForRun refuses partial and failed garden runs before exposing
// anomaly rows to the deterministic correlation pass.
func (s *Store) IncidentSignalsForRun(ctx context.Context, runID uuid.UUID) (JudgmentRun, []IncidentSignal, error) {
	var run JudgmentRun
	err := s.pool.QueryRow(ctx, `SELECT id, started_at, completed_at, expected, succeeded, failed
		FROM judgment_runs WHERE id = $1`, runID).Scan(&run.ID, &run.StartedAt,
		&run.CompletedAt, &run.Expected, &run.Succeeded, &run.Failed)
	if errors.Is(err, pgx.ErrNoRows) {
		return JudgmentRun{}, nil, ErrNotFound
	}
	if err != nil {
		return JudgmentRun{}, nil, err
	}
	if run.CompletedAt == nil || run.Failed != 0 || run.Succeeded != run.Expected {
		return run, nil, fmt.Errorf("%w: incident detection requires a complete successful judgment run", plant.ErrInvalid)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT `+plantColumnsFor("p")+`, v.id, v.action, v.reasoning, v.confidence
		FROM judgment_results jr
		JOIN plants p ON p.id = jr.plant_id
		JOIN LATERAL (
			SELECT id, action, reasoning, confidence FROM verdicts
			WHERE plant_id = p.id AND created_at >= $2 AND created_at <= $3
			ORDER BY created_at DESC LIMIT 1
		) v ON true
		WHERE jr.run_id = $1 AND jr.succeeded = true
		  AND v.action = 'urgent'
		  AND p.archived_at IS NULL
		ORDER BY p.id`, runID, run.StartedAt, *run.CompletedAt)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()
	out := []IncidentSignal{}
	for rows.Next() {
		var signal IncidentSignal
		var ps plantScan
		dest := ps.targets(&signal.Plant)
		dest = append(dest, &signal.VerdictID, &signal.Action, &signal.Reasoning, &signal.Confidence)
		if err := rows.Scan(dest...); err != nil {
			return run, nil, err
		}
		if err := ps.finish(&signal.Plant); err != nil {
			return run, nil, err
		}
		out = append(out, signal)
	}
	return run, out, rows.Err()
}

func (s *Store) IncidentCareSignals(ctx context.Context, run JudgmentRun, plantIDs []uuid.UUID) ([]IncidentCareSignal, error) {
	if run.CompletedAt == nil || len(plantIDs) == 0 {
		return []IncidentCareSignal{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT id, plant_id, kind, occurred_at FROM observations
		WHERE plant_id = ANY($1) AND occurred_at >= $2::timestamptz - interval '24 hours' AND occurred_at <= $3
		  AND kind IN ('watered','airflow','misted','repotted','fertilized','pruned','moved')
		ORDER BY occurred_at`, plantIDs, run.StartedAt, *run.CompletedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IncidentCareSignal{}
	for rows.Next() {
		var signal IncidentCareSignal
		if err := rows.Scan(&signal.ID, &signal.PlantID, &signal.Kind, &signal.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, signal)
	}
	return out, rows.Err()
}

func (s *Store) IncidentEnvironmentFailures(ctx context.Context, run JudgmentRun, areas []string) ([]IncidentEnvironmentFailure, error) {
	if run.CompletedAt == nil || len(areas) == 0 {
		return []IncidentEnvironmentFailure{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT sl.id, sl.zone, sl.ha_entity_id
		FROM sensor_links sl
		LEFT JOIN readings r ON r.sensor_link_id = sl.id
		WHERE sl.zone = ANY($1)
		  AND sl.role IN ('ambient_temp','ambient_humidity','illuminance')
		GROUP BY sl.id, sl.zone, sl.ha_entity_id
		HAVING max(r.taken_at) IS NULL OR max(r.taken_at) < $2
		ORDER BY sl.zone, sl.id`, areas, run.StartedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IncidentEnvironmentFailure{}
	for rows.Next() {
		var failure IncidentEnvironmentFailure
		if err := rows.Scan(&failure.SensorLinkID, &failure.Area, &failure.EntityID); err != nil {
			return nil, err
		}
		out = append(out, failure)
	}
	return out, rows.Err()
}

func (s *Store) IncidentActuatorFailures(ctx context.Context, run JudgmentRun, plantIDs []uuid.UUID) ([]IncidentActuatorFailure, error) {
	if run.CompletedAt == nil || len(plantIDs) == 0 {
		return []IncidentActuatorFailure{}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT e.id, e.actuator_id, ap.plant_id
		FROM plant_actuator_events e
		JOIN plant_actuator_plants ap ON ap.actuator_id = e.actuator_id
		WHERE ap.plant_id = ANY($1)
		  AND e.action IN ('start_failed','stop_failed')
		  AND e.created_at >= $2::timestamptz - interval '24 hours'
		  AND e.created_at <= $3
		ORDER BY e.actuator_id, ap.plant_id, e.id`, plantIDs, run.StartedAt, *run.CompletedAt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IncidentActuatorFailure{}
	for rows.Next() {
		var failure IncidentActuatorFailure
		if err := rows.Scan(&failure.EventID, &failure.ActuatorID, &failure.PlantID); err != nil {
			return nil, err
		}
		out = append(out, failure)
	}
	return out, rows.Err()
}

func (s *Store) UpsertIncidentCandidate(ctx context.Context, candidate plant.IncidentCandidate) (plant.GardenIncident, bool, error) {
	if err := candidate.Valid(); err != nil {
		return plant.GardenIncident{}, false, err
	}
	raw, err := json.Marshal(candidate.Evidence)
	if err != nil {
		return plant.GardenIncident{}, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.GardenIncident{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var complete bool
	if err := tx.QueryRow(ctx, `SELECT completed_at IS NOT NULL AND failed = 0 AND succeeded = expected
		FROM judgment_runs WHERE id = $1 FOR SHARE`, candidate.Evidence.RunID).Scan(&complete); err != nil {
		return plant.GardenIncident{}, false, classify(err)
	}
	if !complete {
		return plant.GardenIncident{}, false, fmt.Errorf("%w: incident detection requires a complete successful judgment run", plant.ErrInvalid)
	}
	if err := validateIncidentCandidateEvidence(ctx, tx, candidate); err != nil {
		return plant.GardenIncident{}, false, err
	}

	var id uuid.UUID
	created := false
	err = tx.QueryRow(ctx, `SELECT id FROM garden_incidents
		WHERE suspected_factor_type = $1 AND suspected_factor_ref = $2 AND status <> 'resolved'
		FOR UPDATE`, candidate.Factor, candidate.FactorRef).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		created = true
		err = tx.QueryRow(ctx, `INSERT INTO garden_incidents
			(suspected_factor_type, suspected_factor_ref, summary, reason, confidence, evidence, detected_run_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, candidate.Factor,
			candidate.FactorRef, candidate.Summary, candidate.Reason, candidate.Confidence, raw, candidate.Evidence.RunID).Scan(&id)
	} else if err == nil {
		_, err = tx.Exec(ctx, `UPDATE garden_incidents SET summary = $2, reason = $3, confidence = $4,
			evidence = $5, detected_run_id = $6, last_seen_at = now(), updated_at = now()
			WHERE id = $1`, id, candidate.Summary, candidate.Reason, candidate.Confidence, raw, candidate.Evidence.RunID)
	}
	if err != nil {
		return plant.GardenIncident{}, false, classify(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO garden_incident_detections
		(incident_id, run_id, confidence, evidence) VALUES ($1,$2,$3,$4)
		ON CONFLICT (incident_id, run_id) DO NOTHING`, id, candidate.Evidence.RunID, candidate.Confidence, raw); err != nil {
		return plant.GardenIncident{}, false, classify(err)
	}
	for _, member := range candidate.Plants {
		memberEvidence, err := json.Marshal(map[string]any{
			"verdict_id": member.VerdictID, "action": member.Action, "confidence": member.Confidence,
		})
		if err != nil {
			return plant.GardenIncident{}, false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO garden_incident_plants
			(incident_id, plant_id, role, evidence) VALUES ($1,$2,'affected',$3)
			ON CONFLICT (incident_id, plant_id) DO UPDATE SET
				evidence = excluded.evidence, last_seen_at = now()`, id, member.Plant.ID, memberEvidence); err != nil {
			return plant.GardenIncident{}, false, classify(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.GardenIncident{}, false, err
	}
	incident, err := s.Incident(ctx, id)
	return incident, created, err
}

func validateIncidentCandidateEvidence(ctx context.Context, tx pgx.Tx, candidate plant.IncidentCandidate) error {
	plantIDs := make([]uuid.UUID, 0, len(candidate.Plants))
	for _, member := range candidate.Plants {
		plantIDs = append(plantIDs, member.Plant.ID)
		var found bool
		err := tx.QueryRow(ctx, `SELECT true FROM verdicts v
			JOIN judgment_results jr ON jr.plant_id = v.plant_id AND jr.run_id = $3 AND jr.succeeded
			JOIN judgment_runs j ON j.id = jr.run_id
			WHERE v.id = $1 AND v.plant_id = $2
			  AND v.created_at >= j.started_at AND v.created_at <= j.completed_at`,
			member.VerdictID, member.Plant.ID, candidate.Evidence.RunID).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: affected verdict is not evidence from this completed run", plant.ErrInvalid)
		}
		if err != nil {
			return err
		}
	}
	checks := []struct {
		name  string
		ids   []uuid.UUID
		query string
	}{
		{"observations", candidate.Evidence.ObservationIDs, `SELECT count(*) FROM observations WHERE id = ANY($1) AND plant_id = ANY($2)`},
		{"actuator events", candidate.Evidence.ActuatorEventIDs, `SELECT count(DISTINCT e.id)
			FROM plant_actuator_events e
			JOIN plant_actuator_plants ap ON ap.actuator_id = e.actuator_id
			WHERE e.id = ANY($1) AND ap.plant_id = ANY($2)`},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		var count int
		if err := tx.QueryRow(ctx, check.query, check.ids, plantIDs).Scan(&count); err != nil {
			return err
		}
		if count != len(check.ids) {
			return fmt.Errorf("%w: incident %s are not inspectable source records", plant.ErrInvalid, check.name)
		}
	}
	if len(candidate.Evidence.SensorLinkIDs) > 0 {
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM sensor_links sl
			WHERE sl.id = ANY($1) AND sl.zone IN (
				SELECT ha_area FROM plants WHERE id = ANY($2) AND ha_area <> ''
			)`, candidate.Evidence.SensorLinkIDs, plantIDs).Scan(&count)
		if err != nil {
			return err
		}
		if count != len(candidate.Evidence.SensorLinkIDs) {
			return fmt.Errorf("%w: environmental evidence is not linked to an affected plant area", plant.ErrInvalid)
		}
	}
	return nil
}

func (s *Store) Incidents(ctx context.Context, status plant.IncidentStatus) ([]plant.GardenIncident, error) {
	query := `SELECT ` + incidentColumns + ` FROM garden_incidents`
	args := []any{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plant.GardenIncident{}
	for rows.Next() {
		incident, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		incident.Plants, err = s.incidentPlants(ctx, incident.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, incident)
	}
	return out, rows.Err()
}

func (s *Store) Incident(ctx context.Context, id uuid.UUID) (plant.GardenIncident, error) {
	incident, err := scanIncident(s.pool.QueryRow(ctx, `SELECT `+incidentColumns+`
		FROM garden_incidents WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.GardenIncident{}, ErrNotFound
	}
	if err != nil {
		return plant.GardenIncident{}, err
	}
	incident.Plants, err = s.incidentPlants(ctx, id)
	return incident, err
}

func (s *Store) AcknowledgeIncident(ctx context.Context, id uuid.UUID, actor string) (plant.GardenIncident, error) {
	if strings.TrimSpace(actor) == "" {
		return plant.GardenIncident{}, fmt.Errorf("%w: acknowledgement actor is required", plant.ErrInvalid)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE garden_incidents SET status = 'acknowledged',
		acknowledged_at = coalesce(acknowledged_at, now()), acknowledged_by = $2, updated_at = now()
		WHERE id = $1 AND status <> 'resolved'`, id, strings.TrimSpace(actor))
	if err != nil {
		return plant.GardenIncident{}, err
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.Incident(ctx, id); err != nil {
			return plant.GardenIncident{}, err
		}
		return plant.GardenIncident{}, fmt.Errorf("%w: resolved incident cannot be acknowledged", plant.ErrInvalid)
	}
	return s.Incident(ctx, id)
}

func (s *Store) ResolveIncident(ctx context.Context, id uuid.UUID, outcome plant.IncidentResolution, actor, conclusion string) (plant.GardenIncident, error) {
	if err := outcome.Valid(); err != nil {
		return plant.GardenIncident{}, err
	}
	if strings.TrimSpace(actor) == "" {
		return plant.GardenIncident{}, fmt.Errorf("%w: resolution actor is required", plant.ErrInvalid)
	}
	tag, err := s.pool.Exec(ctx, `UPDATE garden_incidents SET status = 'resolved',
		resolved_at = now(), resolved_by = $2, resolution = $3, conclusion = $4, updated_at = now()
		WHERE id = $1 AND status <> 'resolved'`, id, strings.TrimSpace(actor), outcome, strings.TrimSpace(conclusion))
	if err != nil {
		return plant.GardenIncident{}, classify(err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := s.Incident(ctx, id); err != nil {
			return plant.GardenIncident{}, err
		}
		return plant.GardenIncident{}, fmt.Errorf("%w: incident is already resolved", plant.ErrInvalid)
	}
	return s.Incident(ctx, id)
}

func scanIncident(row interface{ Scan(...any) error }) (plant.GardenIncident, error) {
	var incident plant.GardenIncident
	var raw []byte
	err := row.Scan(&incident.ID, &incident.Status, &incident.SuspectedFactorType,
		&incident.SuspectedFactorRef, &incident.Summary, &incident.Reason, &incident.Confidence, &raw,
		&incident.DetectedRunID, &incident.FirstSeenAt, &incident.LastSeenAt,
		&incident.AcknowledgedAt, &incident.AcknowledgedBy, &incident.ResolvedAt,
		&incident.ResolvedBy, &incident.Resolution, &incident.Conclusion,
		&incident.CreatedAt, &incident.UpdatedAt)
	if err != nil {
		return plant.GardenIncident{}, err
	}
	if err := json.Unmarshal(raw, &incident.Evidence); err != nil {
		return plant.GardenIncident{}, err
	}
	return incident, nil
}

func (s *Store) incidentPlants(ctx context.Context, incidentID uuid.UUID) ([]plant.IncidentPlant, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+plantColumnsFor("p")+`, ip.role, ip.evidence,
		ip.first_seen_at, ip.last_seen_at FROM garden_incident_plants ip
		JOIN plants p ON p.id = ip.plant_id WHERE ip.incident_id = $1 ORDER BY p.common_name`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []plant.IncidentPlant{}
	for rows.Next() {
		var member plant.IncidentPlant
		var ps plantScan
		var raw []byte
		dest := ps.targets(&member.Plant)
		dest = append(dest, &member.Role, &raw, &member.FirstSeen, &member.LastSeen)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if err := ps.finish(&member.Plant); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &member.Evidence); err != nil {
			return nil, err
		}
		if rawID, ok := member.Evidence["verdict_id"].(string); ok {
			member.VerdictID, _ = uuid.Parse(rawID)
		}
		if action, ok := member.Evidence["action"].(string); ok {
			member.Action = plant.Action(action)
		}
		if confidence, ok := member.Evidence["confidence"].(float64); ok {
			member.Confidence = confidence
		}
		out = append(out, member)
	}
	return out, rows.Err()
}
