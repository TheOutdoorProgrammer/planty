-- +goose Up

CREATE TYPE watering_attempt_outcome AS ENUM (
    'pending',
    'awaiting_evidence',
    'verified',
    'clogged',
    'sensor_unknown',
    'pump_failed',
    'mixed'
);

CREATE TYPE watering_plant_outcome AS ENUM (
    'pending',
    'verified',
    'clogged',
    'sensor_unknown',
    'pump_failed'
);

CREATE TABLE watering_attempts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    requested_at      timestamptz NOT NULL DEFAULT now(),
    pump_started_at   timestamptz,
    pump_stopped_at   timestamptz,
    finalized_at      timestamptz,
    pump_switch       text NOT NULL,
    pump_sensor       text NOT NULL DEFAULT '',
    requested_seconds int NOT NULL CHECK (requested_seconds > 0),
    pump_activity     text NOT NULL DEFAULT 'unknown'
        CHECK (pump_activity IN ('unknown', 'confirmed', 'inactive')),
    outcome           watering_attempt_outcome NOT NULL DEFAULT 'pending',
    start_error       text NOT NULL DEFAULT '',
    stop_error        text NOT NULL DEFAULT '',
    alert_sent_at     timestamptz,
    alert_error       text NOT NULL DEFAULT ''
);

CREATE TABLE watering_attempt_plants (
    attempt_id     uuid NOT NULL REFERENCES watering_attempts (id) ON DELETE CASCADE,
    plant_id       uuid NOT NULL REFERENCES plants (id),
    outcome        watering_plant_outcome NOT NULL DEFAULT 'pending',
    evidence       jsonb NOT NULL DEFAULT '{}'::jsonb,
    observation_id uuid REFERENCES observations (id),
    PRIMARY KEY (attempt_id, plant_id)
);

CREATE INDEX watering_attempts_unfinished
    ON watering_attempts (pump_started_at)
    WHERE outcome = 'awaiting_evidence';

CREATE INDEX watering_attempts_unsent_alerts
    ON watering_attempts (finalized_at)
    WHERE outcome IN ('clogged', 'sensor_unknown', 'pump_failed', 'mixed')
      AND alert_sent_at IS NULL;

-- +goose Down

DROP TABLE watering_attempt_plants;
DROP TABLE watering_attempts;
DROP TYPE watering_plant_outcome;
DROP TYPE watering_attempt_outcome;
