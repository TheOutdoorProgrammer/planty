package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/TheOutdoorProgrammer/planty/internal/plant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const evidenceWindowColumns = `
	id, kind, status, intervention_kind, intervention_observation_id,
	earliest_review_at, latest_review_at, started_at, ready_at, completed_at,
	outcome, conclusion, confounded_at, confound_reason, proposed_by,
	proposed_actor, created_at, updated_at`

type evidenceQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ProposeEvidenceWindow persists a deterministic recheck or experiment in its
// proposed state. Clients cannot provide guardrail contents; those are derived
// from the intervention kind here.
func (s *Store) ProposeEvidenceWindow(ctx context.Context, window plant.EvidenceWindow) (plant.EvidenceWindow, error) {
	now := time.Now().UTC()
	if err := window.ValidProposal(now); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if strings.TrimSpace(window.ProposedActor) == "" {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: proposed_actor is required", plant.ErrInvalid)
	}
	guardrail, _, _, _ := plant.GuardrailFor(window.InterventionKind)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := validateWindowPlants(ctx, tx, window.PlantIDs); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if _, err := validateWindowEvidence(ctx, tx, window.Baseline, now); err != nil {
		return plant.EvidenceWindow{}, err
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO evidence_windows (
			kind, intervention_kind, earliest_review_at, latest_review_at,
			proposed_by, proposed_actor
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`, window.Kind, window.InterventionKind,
		window.EarliestReviewAt, window.LatestReviewAt,
		window.ProposedBy, strings.TrimSpace(window.ProposedActor)).Scan(&window.ID)
	if err != nil {
		return plant.EvidenceWindow{}, classify(err)
	}

	for _, plantID := range window.PlantIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO evidence_window_plants (window_id, plant_id) VALUES ($1,$2)`, window.ID, plantID); err != nil {
			return plant.EvidenceWindow{}, classify(err)
		}
	}
	for _, ref := range window.Baseline {
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_window_refs (window_id, plant_id, kind, evidence_id, phase)
			VALUES ($1,$2,$3,$4,$5)`, window.ID, ref.PlantID, ref.Kind, ref.ID, ref.Phase); err != nil {
			return plant.EvidenceWindow{}, classify(err)
		}
	}
	for _, expected := range window.Expected {
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_window_expectations (window_id, plant_id, kind, instruction)
			VALUES ($1,$2,$3,$4)`, window.ID, expected.PlantID, expected.Kind,
			strings.TrimSpace(expected.Instruction)); err != nil {
			return plant.EvidenceWindow{}, classify(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO evidence_window_guardrails (window_id, reason, conflicting_kinds, red_flags)
		VALUES ($1,$2,$3::text[]::observation_kind[],$4)`, window.ID, guardrail.Reason,
		observationKindStrings(guardrail.ConflictingKinds), guardrail.RedFlags); err != nil {
		return plant.EvidenceWindow{}, classify(err)
	}
	if window.Kind == plant.WindowExperiment {
		experiment := window.Experiment
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_window_experiments (
				window_id, title, hypothesis, variable_kind, variable_value,
				hold_constant_rules, success_criteria
			) VALUES ($1,$2,$3,$4,$5,$6,$7)`, window.ID,
			strings.TrimSpace(experiment.Title), strings.TrimSpace(experiment.Hypothesis),
			strings.TrimSpace(experiment.VariableKind), strings.TrimSpace(experiment.VariableValue),
			trimStrings(experiment.HoldConstantRules), trimStrings(experiment.SuccessCriteria)); err != nil {
			return plant.EvidenceWindow{}, classify(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return plant.EvidenceWindow{}, err
	}
	return s.EvidenceWindow(ctx, window.ID)
}

// EvidenceWindow returns one workflow with its normalized memberships,
// references, expectations, guardrail, experiment, and overrides.
func (s *Store) EvidenceWindow(ctx context.Context, id uuid.UUID) (plant.EvidenceWindow, error) {
	return loadEvidenceWindow(ctx, s.pool, id)
}

// EvidenceWindows lists workflows, optionally narrowed by plant or kind.
func (s *Store) EvidenceWindows(ctx context.Context, plantID *uuid.UUID, kind *plant.EvidenceWindowKind) ([]plant.EvidenceWindow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT w.id
		FROM evidence_windows w
		LEFT JOIN evidence_window_plants p ON p.window_id = w.id
		WHERE ($1::uuid IS NULL OR p.plant_id = $1)
		  AND ($2::text IS NULL OR w.kind::text = $2)
		ORDER BY w.id`, plantID, kindString(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]plant.EvidenceWindow, 0, len(ids))
	for _, id := range ids {
		window, err := s.EvidenceWindow(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, window)
	}
	return out, nil
}

// StartEvidenceWindow binds the proposal to exactly one intervention
// observation. Scheduled automation may propose, but cannot start a window.
func (s *Store) StartEvidenceWindow(ctx context.Context, id, observationID uuid.UUID, source plant.Source, actor string) (plant.EvidenceWindow, error) {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEvidenceWindow(ctx, tx, id); err != nil {
		return plant.EvidenceWindow{}, err
	}
	window, err := loadEvidenceWindow(ctx, tx, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if err := window.CanStart(source, observationID, now); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if strings.TrimSpace(actor) == "" {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: starter actor is required", plant.ErrInvalid)
	}

	var interventionPlant uuid.UUID
	var interventionKind plant.ObservationKind
	var interventionAt time.Time
	err = tx.QueryRow(ctx, `
		SELECT o.plant_id, o.kind, o.occurred_at
		FROM observations o
		JOIN evidence_window_plants p ON p.plant_id = o.plant_id AND p.window_id = $1
		WHERE o.id = $2`, id, observationID).
		Scan(&interventionPlant, &interventionKind, &interventionAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: intervention observation does not belong to a window plant", plant.ErrInvalid)
	}
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if interventionKind != window.InterventionKind {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: intervention observation is %q, want %q", plant.ErrInvalid, interventionKind, window.InterventionKind)
	}
	baselineAt, err := validateWindowEvidence(ctx, tx, window.Baseline, now)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if interventionAt.Before(baselineAt) {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: intervention predates the baseline evidence", plant.ErrInvalid)
	}
	_ = interventionPlant

	_, err = tx.Exec(ctx, `
		UPDATE evidence_windows
		SET status = 'active', intervention_observation_id = $2,
		    started_at = $3, started_by = $4, started_actor = $5, updated_at = now()
		WHERE id = $1`, id, observationID, now, source, strings.TrimSpace(actor))
	if err != nil {
		return plant.EvidenceWindow{}, classify(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.EvidenceWindow{}, err
	}
	return s.EvidenceWindow(ctx, id)
}

// MarkEvidenceWindowReady links complete, fresh review evidence and advances
// the shared state machine. Interpretation happens only after this transition.
func (s *Store) MarkEvidenceWindowReady(ctx context.Context, id uuid.UUID, refs []plant.EvidenceRef) (plant.EvidenceWindow, error) {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEvidenceWindow(ctx, tx, id); err != nil {
		return plant.EvidenceWindow{}, err
	}
	window, err := loadEvidenceWindow(ctx, tx, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if err := window.CanMarkReady(refs, now); err != nil {
		return plant.EvidenceWindow{}, err
	}
	newest, err := validateWindowEvidence(ctx, tx, refs, now)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if window.StartedAt == nil || !newest.After(*window.StartedAt) {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: review evidence must be newer than the intervention", plant.ErrInvalid)
	}
	for _, ref := range refs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_window_refs (window_id, plant_id, kind, evidence_id, phase)
			VALUES ($1,$2,$3,$4,$5)`, id, ref.PlantID, ref.Kind, ref.ID, ref.Phase); err != nil {
			return plant.EvidenceWindow{}, classify(err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE evidence_windows SET status = 'ready', ready_at = $2, updated_at = now()
		WHERE id = $1`, id, now); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.EvidenceWindow{}, err
	}
	return s.EvidenceWindow(ctx, id)
}

// ConcludeEvidenceWindow records the evidence-linked terminal interpretation.
func (s *Store) ConcludeEvidenceWindow(ctx context.Context, id uuid.UUID, outcome plant.EvidenceWindowOutcome, conclusion string, source plant.Source, actor string) (plant.EvidenceWindow, error) {
	now := time.Now().UTC()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockEvidenceWindow(ctx, tx, id); err != nil {
		return plant.EvidenceWindow{}, err
	}
	window, err := loadEvidenceWindow(ctx, tx, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if err := window.CanConclude(outcome, conclusion); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if err := validWorkflowActor(source, actor); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE evidence_windows
		SET status = 'completed', outcome = $2, conclusion = $3,
		    completed_at = $4, completed_by = $5, completed_actor = $6, updated_at = now()
		WHERE id = $1`, id, outcome, strings.TrimSpace(conclusion), now, source, strings.TrimSpace(actor)); err != nil {
		return plant.EvidenceWindow{}, classify(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.EvidenceWindow{}, err
	}
	return s.EvidenceWindow(ctx, id)
}

// CancelEvidenceWindow is terminal but makes no claim about the intervention.
func (s *Store) CancelEvidenceWindow(ctx context.Context, id uuid.UUID, source plant.Source, actor, reason string) (plant.EvidenceWindow, error) {
	if err := validWorkflowActor(source, actor); err != nil {
		return plant.EvidenceWindow{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: cancellation reason is required", plant.ErrInvalid)
	}
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE evidence_windows
		SET status = 'cancelled', outcome = 'cancelled', conclusion = $2,
		    completed_at = $3, completed_by = $4, completed_actor = $5, updated_at = now()
		WHERE id = $1 AND status IN ('proposed', 'active', 'ready')`,
		id, strings.TrimSpace(reason), now, source, strings.TrimSpace(actor))
	if err != nil {
		return plant.EvidenceWindow{}, classify(err)
	}
	if tag.RowsAffected() == 0 {
		return plant.EvidenceWindow{}, fmt.Errorf("%w: evidence window is missing or terminal", plant.ErrInvalid)
	}
	return s.EvidenceWindow(ctx, id)
}

// OverrideGuardrail records a deliberate conflict and marks the shared window
// confounded. It does not create or block an observation.
func (s *Store) OverrideGuardrail(ctx context.Context, override plant.GuardrailOverride) (plant.GuardrailOverride, error) {
	if err := override.Valid(); err != nil {
		return plant.GuardrailOverride{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return plant.GuardrailOverride{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockEvidenceWindow(ctx, tx, override.WindowID); err != nil {
		return plant.GuardrailOverride{}, err
	}
	window, err := loadEvidenceWindow(ctx, tx, override.WindowID)
	if err != nil {
		return plant.GuardrailOverride{}, err
	}
	if window.Status != plant.WindowActive && window.Status != plant.WindowReady {
		return plant.GuardrailOverride{}, fmt.Errorf("%w: only an active guardrail can be overridden", plant.ErrInvalid)
	}
	if !slices.Contains(window.PlantIDs, override.PlantID) {
		return plant.GuardrailOverride{}, fmt.Errorf("%w: override plant is not in the evidence window", plant.ErrInvalid)
	}
	if window.Guardrail == nil || !slices.Contains(window.Guardrail.ConflictingKinds, override.Kind) {
		return plant.GuardrailOverride{}, fmt.Errorf("%w: %q does not conflict with this guardrail", plant.ErrInvalid, override.Kind)
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO evidence_window_guardrail_overrides (
			window_id, plant_id, kind, reason, source, actor
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at`, override.WindowID, override.PlantID, override.Kind,
		strings.TrimSpace(override.Reason), override.Source, strings.TrimSpace(override.Actor)).
		Scan(&override.ID, &override.CreatedAt)
	if err != nil {
		return plant.GuardrailOverride{}, classify(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE evidence_windows
		SET confounded_at = coalesce(confounded_at, $2),
		    confound_reason = CASE WHEN confound_reason = '' THEN $3 ELSE confound_reason END,
		    updated_at = now()
		WHERE id = $1`, override.WindowID, override.CreatedAt,
		"guardrail overridden for "+string(override.Kind)); err != nil {
		return plant.GuardrailOverride{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return plant.GuardrailOverride{}, err
	}
	return override, nil
}

func loadEvidenceWindow(ctx context.Context, q evidenceQuerier, id uuid.UUID) (plant.EvidenceWindow, error) {
	window := plant.EvidenceWindow{
		PlantIDs:  []uuid.UUID{},
		Baseline:  []plant.EvidenceRef{},
		Expected:  []plant.EvidenceExpectation{},
		Review:    []plant.EvidenceRef{},
		Overrides: []plant.GuardrailOverride{},
	}
	var outcome *string
	err := q.QueryRow(ctx, `SELECT `+evidenceWindowColumns+` FROM evidence_windows WHERE id = $1`, id).
		Scan(&window.ID, &window.Kind, &window.Status, &window.InterventionKind,
			&window.InterventionObservationID, &window.EarliestReviewAt, &window.LatestReviewAt,
			&window.StartedAt, &window.ReadyAt, &window.CompletedAt, &outcome,
			&window.Conclusion, &window.ConfoundedAt, &window.ConfoundReason,
			&window.ProposedBy, &window.ProposedActor, &window.CreatedAt, &window.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return plant.EvidenceWindow{}, ErrNotFound
	}
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	if outcome != nil {
		value := plant.EvidenceWindowOutcome(*outcome)
		window.Outcome = &value
	}

	rows, err := q.Query(ctx, `SELECT plant_id FROM evidence_window_plants WHERE window_id = $1 ORDER BY plant_id`, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	for rows.Next() {
		var plantID uuid.UUID
		if err := rows.Scan(&plantID); err != nil {
			rows.Close()
			return plant.EvidenceWindow{}, err
		}
		window.PlantIDs = append(window.PlantIDs, plantID)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT plant_id, kind, evidence_id, phase
		FROM evidence_window_refs WHERE window_id = $1
		ORDER BY phase, kind, evidence_id`, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	for rows.Next() {
		var ref plant.EvidenceRef
		if err := rows.Scan(&ref.PlantID, &ref.Kind, &ref.ID, &ref.Phase); err != nil {
			rows.Close()
			return plant.EvidenceWindow{}, err
		}
		if ref.Phase == plant.EvidenceBaseline {
			window.Baseline = append(window.Baseline, ref)
		} else {
			window.Review = append(window.Review, ref)
		}
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		SELECT plant_id, kind, instruction
		FROM evidence_window_expectations WHERE window_id = $1
		ORDER BY plant_id, kind`, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	for rows.Next() {
		var expected plant.EvidenceExpectation
		if err := rows.Scan(&expected.PlantID, &expected.Kind, &expected.Instruction); err != nil {
			rows.Close()
			return plant.EvidenceWindow{}, err
		}
		window.Expected = append(window.Expected, expected)
	}
	rows.Close()

	var guardrail plant.Guardrail
	var conflictingKinds []string
	err = q.QueryRow(ctx, `
		SELECT reason, conflicting_kinds, red_flags
		FROM evidence_window_guardrails WHERE window_id = $1`, id).
		Scan(&guardrail.Reason, &conflictingKinds, &guardrail.RedFlags)
	if err == nil {
		guardrail.ConflictingKinds = make([]plant.ObservationKind, len(conflictingKinds))
		for i, kind := range conflictingKinds {
			guardrail.ConflictingKinds[i] = plant.ObservationKind(kind)
		}
		window.Guardrail = &guardrail
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return plant.EvidenceWindow{}, err
	}

	var experiment plant.Experiment
	err = q.QueryRow(ctx, `
		SELECT title, hypothesis, variable_kind, variable_value,
		       hold_constant_rules, success_criteria
		FROM evidence_window_experiments WHERE window_id = $1`, id).
		Scan(&experiment.Title, &experiment.Hypothesis,
			&experiment.VariableKind, &experiment.VariableValue,
			&experiment.HoldConstantRules, &experiment.SuccessCriteria)
	if err == nil {
		window.Experiment = &experiment
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return plant.EvidenceWindow{}, err
	}

	rows, err = q.Query(ctx, `
		SELECT id, window_id, plant_id, kind, reason, source, actor, created_at
		FROM evidence_window_guardrail_overrides WHERE window_id = $1
		ORDER BY created_at, id`, id)
	if err != nil {
		return plant.EvidenceWindow{}, err
	}
	for rows.Next() {
		var override plant.GuardrailOverride
		if err := rows.Scan(&override.ID, &override.WindowID, &override.PlantID,
			&override.Kind, &override.Reason, &override.Source, &override.Actor,
			&override.CreatedAt); err != nil {
			rows.Close()
			return plant.EvidenceWindow{}, err
		}
		window.Overrides = append(window.Overrides, override)
	}
	rows.Close()
	return window, rows.Err()
}

func lockEvidenceWindow(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var found uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM evidence_windows WHERE id = $1 FOR UPDATE`, id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func validateWindowPlants(ctx context.Context, q evidenceQuerier, ids []uuid.UUID) error {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM plants WHERE id = ANY($1) AND archived_at IS NULL`, ids).Scan(&count); err != nil {
		return err
	}
	if count != len(ids) {
		return fmt.Errorf("%w: evidence window includes an unknown or archived plant", plant.ErrInvalid)
	}
	return nil
}

func validateWindowEvidence(ctx context.Context, q evidenceQuerier, refs []plant.EvidenceRef, notAfter time.Time) (time.Time, error) {
	var newest time.Time
	seen := map[string]bool{}
	for _, ref := range refs {
		if err := ref.Valid(); err != nil {
			return time.Time{}, err
		}
		key := string(ref.Phase) + "/" + string(ref.Kind) + "/" + ref.ID.String()
		if seen[key] {
			return time.Time{}, fmt.Errorf("%w: duplicate evidence reference %s", plant.ErrInvalid, ref.ID)
		}
		seen[key] = true

		var owner uuid.UUID
		var at time.Time
		var err error
		switch ref.Kind {
		case plant.EvidenceObservation:
			err = q.QueryRow(ctx, `SELECT plant_id, occurred_at FROM observations WHERE id = $1`, ref.ID).Scan(&owner, &at)
		case plant.EvidencePhoto:
			err = q.QueryRow(ctx, `SELECT plant_id, taken_at FROM photos WHERE id = $1 AND deletion_requested_at IS NULL`, ref.ID).Scan(&owner, &at)
		case plant.EvidenceReading:
			err = q.QueryRow(ctx, `
				SELECT s.plant_id, r.taken_at
				FROM readings r JOIN sensor_links s ON s.id = r.sensor_link_id
				WHERE r.id = $1 AND s.plant_id IS NOT NULL`, ref.ID).Scan(&owner, &at)
		}
		if errors.Is(err, pgx.ErrNoRows) || owner != ref.PlantID {
			return time.Time{}, fmt.Errorf("%w: %s evidence %s does not belong to plant %s", plant.ErrInvalid, ref.Kind, ref.ID, ref.PlantID)
		}
		if err != nil {
			return time.Time{}, err
		}
		if at.After(notAfter) {
			return time.Time{}, fmt.Errorf("%w: evidence %s is dated in the future", plant.ErrInvalid, ref.ID)
		}
		if at.After(newest) {
			newest = at
		}
	}
	return newest, nil
}

func validWorkflowActor(source plant.Source, actor string) error {
	switch source {
	case plant.SourceApp, plant.SourceAgent, plant.SourceAutomation:
	default:
		return fmt.Errorf("%w: unknown workflow source %q", plant.ErrInvalid, source)
	}
	if strings.TrimSpace(actor) == "" {
		return fmt.Errorf("%w: workflow actor is required", plant.ErrInvalid)
	}
	return nil
}

func trimStrings(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.TrimSpace(value)
	}
	return out
}

func observationKindStrings(kinds []plant.ObservationKind) []string {
	out := make([]string, len(kinds))
	for i, kind := range kinds {
		out[i] = string(kind)
	}
	return out
}

func kindString(kind *plant.EvidenceWindowKind) any {
	if kind == nil {
		return nil
	}
	return string(*kind)
}
