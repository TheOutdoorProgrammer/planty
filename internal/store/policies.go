package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TheOutdoorProgrammer/planty/internal/policy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const policyColumns = `id, name, description, source, mode, enabled, version,
	created_at, updated_at, archived_at`

func (s *Store) Policies(ctx context.Context) ([]policy.Policy, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+policyColumns+` FROM opa_policies
		WHERE archived_at IS NULL ORDER BY lower(name), id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []policy.Policy{}
	for rows.Next() {
		item, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) Policy(ctx context.Context, id uuid.UUID) (policy.Policy, error) {
	item, err := scanPolicy(s.pool.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM opa_policies WHERE id = $1 AND archived_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return policy.Policy{}, ErrNotFound
	}
	return item, err
}

func (s *Store) CreatePolicy(ctx context.Context, item policy.Policy) (policy.Policy, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	if err := item.Valid(); err != nil {
		return policy.Policy{}, err
	}
	created, err := scanPolicy(s.pool.QueryRow(ctx, `
		INSERT INTO opa_policies (name, description, source, mode, enabled)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+policyColumns,
		item.Name, item.Description, item.Source, item.Mode, item.Enabled))
	if err != nil {
		return policy.Policy{}, classify(err)
	}
	return created, nil
}

func (s *Store) UpdatePolicy(ctx context.Context, id uuid.UUID, item policy.Policy) (policy.Policy, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	if err := item.Valid(); err != nil {
		return policy.Policy{}, err
	}
	updated, err := scanPolicy(s.pool.QueryRow(ctx, `
		UPDATE opa_policies SET name = $2, description = $3, source = $4,
			mode = $5, enabled = $6, version = version + 1, updated_at = now()
		WHERE id = $1 AND archived_at IS NULL RETURNING `+policyColumns,
		id, item.Name, item.Description, item.Source, item.Mode, item.Enabled))
	if errors.Is(err, pgx.ErrNoRows) {
		return policy.Policy{}, ErrNotFound
	}
	if err != nil {
		return policy.Policy{}, classify(err)
	}
	return updated, nil
}

func (s *Store) ArchivePolicy(ctx context.Context, id uuid.UUID) error {
	command, err := s.pool.Exec(ctx, `UPDATE opa_policies
		SET enabled = false, archived_at = now(), updated_at = now()
		WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SavePolicyEvaluation(ctx context.Context, evaluation policy.Evaluation) (policy.Evaluation, bool, error) {
	input, err := json.Marshal(evaluation.Input)
	if err != nil {
		return policy.Evaluation{}, false, err
	}
	result, err := json.Marshal(evaluation.Result)
	if err != nil {
		return policy.Evaluation{}, false, err
	}
	enforced, err := json.Marshal(evaluation.Enforced)
	if err != nil {
		return policy.Evaluation{}, false, err
	}
	row := s.pool.QueryRow(ctx, `
		INSERT INTO opa_policy_evaluations
			(policy_id, policy_version, policy_mode, plant_id, trigger, input_fingerprint,
			 idempotency_key, policy_fingerprint, input, result, duration_ms, outcome, error, enforced)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (policy_id, policy_version, plant_id, trigger, idempotency_key)
		DO NOTHING
		RETURNING id, created_at`, evaluation.PolicyID, evaluation.PolicyVersion,
		evaluation.PolicyMode, evaluation.PlantID, evaluation.Trigger, evaluation.InputFingerprint,
		evaluation.IdempotencyKey, evaluation.PolicyFingerprint, input, result, evaluation.DurationMS,
		evaluation.Outcome, evaluation.Error, enforced)
	if err := row.Scan(&evaluation.ID, &evaluation.CreatedAt); err == nil {
		return evaluation, true, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return policy.Evaluation{}, false, err
	}

	existing, err := s.policyEvaluationByKey(ctx, evaluation)
	return existing, false, err
}

func (s *Store) PolicyEvaluations(ctx context.Context, plantID *uuid.UUID, limit int) ([]policy.Evaluation, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("policy evaluation limit must be between 1 and 200")
	}
	rows, err := s.pool.Query(ctx, `SELECT id, policy_id, policy_version, policy_mode, plant_id, trigger,
		input_fingerprint, idempotency_key, policy_fingerprint, input, result, duration_ms, outcome,
		error, enforced, created_at FROM opa_policy_evaluations
		WHERE ($1::uuid IS NULL OR plant_id = $1)
		ORDER BY created_at DESC, id DESC LIMIT $2`, plantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []policy.Evaluation{}
	for rows.Next() {
		evaluation, err := scanPolicyEvaluation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, evaluation)
	}
	return out, rows.Err()
}

func (s *Store) FinishPolicyEvaluation(ctx context.Context, id uuid.UUID, outcome, failure string, enforced []string) error {
	raw, err := json.Marshal(enforced)
	if err != nil {
		return err
	}
	command, err := s.pool.Exec(ctx, `UPDATE opa_policy_evaluations
		SET outcome = $2, error = $3, enforced = $4 WHERE id = $1`, id, outcome, failure, raw)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) policyEvaluationByKey(ctx context.Context, key policy.Evaluation) (policy.Evaluation, error) {
	return scanPolicyEvaluation(s.pool.QueryRow(ctx, `SELECT id, policy_id, policy_version, policy_mode, plant_id, trigger,
		input_fingerprint, idempotency_key, policy_fingerprint, input, result, duration_ms, outcome,
		error, enforced, created_at FROM opa_policy_evaluations
		WHERE policy_id = $1 AND policy_version = $2 AND plant_id = $3
			AND trigger = $4 AND idempotency_key = $5`, key.PolicyID,
		key.PolicyVersion, key.PlantID, key.Trigger, key.IdempotencyKey))
}

func scanPolicy(row interface{ Scan(...any) error }) (policy.Policy, error) {
	var item policy.Policy
	if err := row.Scan(&item.ID, &item.Name, &item.Description, &item.Source, &item.Mode,
		&item.Enabled, &item.Version, &item.CreatedAt, &item.UpdatedAt, &item.ArchivedAt); err != nil {
		return policy.Policy{}, err
	}
	return item, nil
}

func scanPolicyEvaluation(row interface{ Scan(...any) error }) (policy.Evaluation, error) {
	var evaluation policy.Evaluation
	var input, result, enforced []byte
	if err := row.Scan(&evaluation.ID, &evaluation.PolicyID, &evaluation.PolicyVersion, &evaluation.PolicyMode,
		&evaluation.PlantID, &evaluation.Trigger, &evaluation.InputFingerprint,
		&evaluation.IdempotencyKey, &evaluation.PolicyFingerprint, &input, &result, &evaluation.DurationMS,
		&evaluation.Outcome, &evaluation.Error, &enforced, &evaluation.CreatedAt); err != nil {
		return policy.Evaluation{}, err
	}
	if err := json.Unmarshal(input, &evaluation.Input); err != nil {
		return policy.Evaluation{}, err
	}
	if err := json.Unmarshal(result, &evaluation.Result); err != nil {
		return policy.Evaluation{}, err
	}
	if err := json.Unmarshal(enforced, &evaluation.Enforced); err != nil {
		return policy.Evaluation{}, err
	}
	return evaluation, nil
}
