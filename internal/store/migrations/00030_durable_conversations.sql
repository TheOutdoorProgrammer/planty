-- +goose Up

ALTER TABLE diagnosis_turns
    ALTER COLUMN reply DROP NOT NULL,
    ADD COLUMN status text NOT NULL DEFAULT 'complete'
        CHECK (status IN ('pending', 'processing', 'complete', 'failed')),
    ADD COLUMN failure text,
    ADD COLUMN attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN lease_id uuid,
    ADD COLUMN lease_expires_at timestamptz,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX conversation_work
    ON diagnosis_turns (created_at)
    WHERE kind = 'consult' AND status IN ('pending', 'processing');

-- +goose Down

DROP INDEX conversation_work;

UPDATE diagnosis_turns SET reply = '{}' WHERE reply IS NULL;

ALTER TABLE diagnosis_turns
    DROP COLUMN updated_at,
    DROP COLUMN lease_expires_at,
    DROP COLUMN lease_id,
    DROP COLUMN attempts,
    DROP COLUMN failure,
    DROP COLUMN status,
    ALTER COLUMN reply SET NOT NULL;
