-- +goose Up

-- Misting is not watering: it wets leaves rather than soil, a moisture probe
-- never sees it, and the two are scheduled on completely different intervals.
ALTER TYPE observation_kind ADD VALUE IF NOT EXISTS 'misted';

-- A reminder is the standing intent and the observation is the evidence it was
-- acted on, so due dates come from observations rather than from whether a
-- notification went out. A notification nobody acted on must not move the
-- schedule forward.
CREATE TABLE reminders (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id     uuid             NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    kind         observation_kind NOT NULL,
    every_days   int              NOT NULL DEFAULT 1,
    at_hours     int[]            NOT NULL DEFAULT '{8}',
    active       bool             NOT NULL DEFAULT true,
    last_sent_at timestamptz,
    note         text             NOT NULL DEFAULT '',
    created_at   timestamptz      NOT NULL DEFAULT now(),

    CONSTRAINT reminder_interval_sane CHECK (every_days BETWEEN 1 AND 365),
    -- An array so a mushroom kit can be misted at 8 and again at 20. A single
    -- hour column cannot say that, and misting twice a day is the common case.
    CONSTRAINT reminder_has_hours CHECK (cardinality(at_hours) BETWEEN 1 AND 24),
    CONSTRAINT reminder_hours_real CHECK (0 <= ALL (at_hours) AND 23 >= ALL (at_hours)),
    CONSTRAINT reminder_one_per_kind UNIQUE (plant_id, kind)
);

CREATE INDEX reminders_due ON reminders (plant_id) WHERE active;

-- +goose Down

DROP TABLE reminders;
