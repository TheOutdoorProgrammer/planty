-- +goose Up

ALTER TYPE plant_status RENAME VALUE 'gone' TO 'removed';
ALTER TYPE plant_actuator_kind ADD VALUE IF NOT EXISTS 'light';
ALTER TYPE plant_actuator_event_action ADD VALUE IF NOT EXISTS 'state_changed';
ALTER TYPE plant_actuator_event_action ADD VALUE IF NOT EXISTS 'schedule_updated';
ALTER TYPE plant_actuator_event_action ADD VALUE IF NOT EXISTS 'schedule_disabled';
ALTER TYPE plant_actuator_event_action ADD VALUE IF NOT EXISTS 'schedule_failed';

CREATE TABLE plant_lineage (
    child_plant_id  uuid PRIMARY KEY REFERENCES plants (id) ON DELETE CASCADE,
    source_plant_id uuid NOT NULL REFERENCES plants (id),
    derived_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT plant_lineage_not_self CHECK (child_plant_id <> source_plant_id)
);

CREATE INDEX plant_lineage_source ON plant_lineage (source_plant_id, derived_at);

CREATE TABLE plant_light_schedules (
    actuator_id        uuid PRIMARY KEY REFERENCES plant_actuators (id) ON DELETE CASCADE,
    start_minute       int NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute         int NOT NULL CHECK (end_minute BETWEEN 0 AND 1439),
    timezone           text NOT NULL CHECK (length(trim(timezone)) > 0),
    enabled            boolean NOT NULL DEFAULT true,
    last_applied_state boolean,
    last_applied_at    timestamptz,
    last_error         text NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT plant_light_schedule_has_duration CHECK (start_minute <> end_minute)
);

-- +goose Down

DROP TABLE plant_light_schedules;
DROP TABLE plant_lineage;
ALTER TYPE plant_status RENAME VALUE 'removed' TO 'gone';
