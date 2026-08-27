-- +goose Up

ALTER TABLE opa_policy_evaluations RENAME COLUMN decision TO result;

-- +goose Down

ALTER TABLE opa_policy_evaluations RENAME COLUMN result TO decision;
