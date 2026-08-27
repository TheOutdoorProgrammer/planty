-- +goose Up

ALTER TABLE plant_actuators
    ADD COLUMN policy_control_enabled boolean NOT NULL DEFAULT false;

CREATE TABLE opa_policies (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL CHECK (length(trim(name)) > 0),
    description text NOT NULL DEFAULT '',
    source      text NOT NULL CHECK (length(source) BETWEEN 1 AND 65536),
    mode        text NOT NULL CHECK (mode IN ('advisory', 'enforce')),
    enabled     boolean NOT NULL DEFAULT false,
    version     int NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz
);

CREATE UNIQUE INDEX opa_policies_active_name
    ON opa_policies (lower(name)) WHERE archived_at IS NULL;

CREATE TABLE opa_policy_evaluations (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    policy_id           uuid NOT NULL REFERENCES opa_policies (id),
    policy_version      int NOT NULL,
    policy_mode         text NOT NULL CHECK (policy_mode IN ('advisory', 'enforce')),
    plant_id            uuid NOT NULL REFERENCES plants (id),
    trigger             text NOT NULL CHECK (trigger IN ('manual', 'daily', 'agent')),
    input_fingerprint   text NOT NULL,
    idempotency_key     text NOT NULL,
    policy_fingerprint  text NOT NULL,
    input               jsonb NOT NULL,
    decision            jsonb NOT NULL,
    duration_ms         double precision NOT NULL CHECK (duration_ms >= 0),
    outcome             text NOT NULL CHECK (outcome IN ('advisory', 'enforced', 'failed')),
    error               text NOT NULL DEFAULT '',
    enforced            jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX opa_policy_evaluations_dedupe
    ON opa_policy_evaluations (policy_id, policy_version, plant_id, trigger, idempotency_key);
CREATE INDEX opa_policy_evaluations_history
    ON opa_policy_evaluations (plant_id, created_at DESC);

-- +goose Down

DROP TABLE opa_policy_evaluations;
DROP TABLE opa_policies;
ALTER TABLE plant_actuators DROP COLUMN policy_control_enabled;
