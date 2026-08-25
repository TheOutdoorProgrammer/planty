-- +goose Up

-- Health is an assessment history, not mutable plant metadata. Keeping every
-- event makes a model's change explainable and keeps a correction from erasing
-- the bad assessment that prompted it.
CREATE TABLE plant_health_events (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id          uuid NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    score             numeric NOT NULL CHECK (score BETWEEN 0 AND 100),
    requested_delta   numeric,
    applied_delta     numeric,
    rationale         text NOT NULL CHECK (length(trim(rationale)) > 0),
    evidence          jsonb NOT NULL,
    source            observation_source NOT NULL,
    actor             text NOT NULL DEFAULT '',
    judgment_run_id   uuid REFERENCES judgment_runs (id),
    idempotency_key   uuid,
    created_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT health_change_shape CHECK (
        (requested_delta IS NULL AND applied_delta IS NULL) OR
        (requested_delta IS NOT NULL AND requested_delta <> 0 AND
         applied_delta IS NOT NULL AND applied_delta <> 0 AND
         requested_delta * applied_delta > 0 AND
         abs(applied_delta) <= abs(requested_delta))
    ),
    CONSTRAINT health_evidence_object CHECK (jsonb_typeof(evidence) = 'object')
);

-- A null delta is the one absolute baseline. Later corrections are explicit
-- signed changes so the audit trail never silently resets its origin.
CREATE UNIQUE INDEX plant_health_one_baseline
    ON plant_health_events (plant_id)
    WHERE requested_delta IS NULL;

-- Retrying one daily run cannot move the same plant twice.
CREATE UNIQUE INDEX plant_health_one_per_judgment
    ON plant_health_events (judgment_run_id, plant_id)
    WHERE judgment_run_id IS NOT NULL;

CREATE UNIQUE INDEX plant_health_idempotency
    ON plant_health_events (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX plant_health_history
    ON plant_health_events (plant_id, created_at DESC, id DESC);

-- +goose Down

DROP TABLE plant_health_events;
