-- +goose Up

CREATE TYPE plant_actuator_kind AS ENUM ('fan', 'switch');
CREATE TYPE plant_actuator_event_action AS ENUM (
    'start_requested', 'started', 'start_failed',
    'stop_requested', 'stopped', 'stop_failed', 'stop_noop'
);

CREATE TABLE plant_actuators (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id   text NOT NULL,
    name        text NOT NULL CHECK (length(trim(name)) > 0),
    kind        plant_actuator_kind NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    CONSTRAINT actuator_entity_matches_kind CHECK (
        (kind = 'fan' AND entity_id ~ '^fan[.][a-z0-9_]+$') OR
        (kind = 'switch' AND entity_id ~ '^switch[.][a-z0-9_]+$')
    )
);

CREATE UNIQUE INDEX plant_actuators_active_entity
    ON plant_actuators (entity_id) WHERE deleted_at IS NULL;

CREATE TABLE plant_actuator_leases (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actuator_id         uuid NOT NULL REFERENCES plant_actuators (id),
    requested_seconds   int NOT NULL CHECK (requested_seconds BETWEEN 1 AND 3600),
    deadline            timestamptz NOT NULL,
    actor               text NOT NULL,
    source              observation_source NOT NULL,
    idempotency_key     uuid NOT NULL UNIQUE,
    started_at          timestamptz,
    stopped_at          timestamptz,
    stop_reason         text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX plant_actuator_one_active_lease
    ON plant_actuator_leases (actuator_id) WHERE stopped_at IS NULL;
CREATE INDEX plant_actuator_overdue_leases
    ON plant_actuator_leases (deadline) WHERE stopped_at IS NULL;

CREATE TABLE plant_actuator_events (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actuator_id         uuid NOT NULL REFERENCES plant_actuators (id),
    lease_id            uuid REFERENCES plant_actuator_leases (id),
    action              plant_actuator_event_action NOT NULL,
    actor               text NOT NULL,
    source              observation_source NOT NULL,
    idempotency_key     uuid,
    detail              text NOT NULL DEFAULT '',
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX plant_actuator_event_idempotency
    ON plant_actuator_events (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX plant_actuator_events_history
    ON plant_actuator_events (actuator_id, created_at DESC);

-- +goose Down

DROP TABLE plant_actuator_events;
DROP TABLE plant_actuator_leases;
DROP TABLE plant_actuators;
DROP TYPE plant_actuator_event_action;
DROP TYPE plant_actuator_kind;
