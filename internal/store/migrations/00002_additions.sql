-- +goose Up

-- Questions only the plant's owner can answer, batched so one text goes out
-- instead of ten.
CREATE TYPE question_status AS ENUM ('open', 'answered', 'dropped');

CREATE TABLE questions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id    uuid REFERENCES plants (id) ON DELETE CASCADE,
    asked_of    text            NOT NULL,
    question    text            NOT NULL,
    why         text,
    status      question_status NOT NULL DEFAULT 'open',
    answer      text,
    answered_at timestamptz,
    created_at  timestamptz     NOT NULL DEFAULT now()
);

CREATE INDEX questions_open ON questions (asked_of) WHERE status = 'open';

-- Away periods change behaviour rather than muting: escalation goes to a backup
-- human, because nagging a phone nobody is holding protects nothing.
CREATE TABLE away_periods (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    starts_at      timestamptz NOT NULL,
    ends_at        timestamptz NOT NULL,
    backup_contact text,
    backup_notify  text,
    note           text,
    created_at     timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT away_period_ordered CHECK (ends_at > starts_at)
);

CREATE INDEX away_periods_window ON away_periods (starts_at, ends_at);

-- What killed a plant, written once when it dies. The point of archiving rather
-- than deleting is that this can be written at all.
CREATE TABLE postmortems (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id     uuid        NOT NULL UNIQUE REFERENCES plants (id) ON DELETE CASCADE,
    likely_cause text        NOT NULL,
    narrative    text        NOT NULL,
    evidence     jsonb       NOT NULL DEFAULT '{}'::jsonb,
    lesson       text,
    created_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE postmortems;
DROP TABLE away_periods;
DROP TABLE questions;
DROP TYPE question_status;
