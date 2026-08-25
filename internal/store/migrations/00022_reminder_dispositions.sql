-- +goose Up

CREATE TYPE reminder_disposition AS ENUM ('completed', 'missed');

ALTER TABLE reminder_completions
    ADD COLUMN disposition reminder_disposition NOT NULL DEFAULT 'completed',
    ADD COLUMN note text NOT NULL DEFAULT '',
    ADD COLUMN responded_at timestamptz;

UPDATE reminder_completions SET responded_at = created_at;

ALTER TABLE reminder_completions
    ALTER COLUMN responded_at SET NOT NULL,
    ALTER COLUMN responded_at SET DEFAULT now(),
    ADD CONSTRAINT reminder_disposition_observation CHECK (
        (disposition = 'completed' AND observation_id IS NOT NULL)
        OR (disposition = 'missed' AND observation_id IS NULL)
    );

-- +goose Down

ALTER TABLE reminder_completions
    DROP CONSTRAINT reminder_disposition_observation,
    DROP COLUMN responded_at,
    DROP COLUMN note,
    DROP COLUMN disposition;

DROP TYPE reminder_disposition;

