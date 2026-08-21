-- +goose Up

-- A recurring reminder can have more than one occurrence per day. Persist the
-- scheduled slot beside the idempotency key so retrying a tap, or tapping from
-- two phones, writes one matching care observation and never suppresses a later
-- occurrence of the same reminder.
CREATE TABLE reminder_completions (
    idempotency_key uuid PRIMARY KEY,
    reminder_id     uuid NOT NULL REFERENCES reminders (id) ON DELETE CASCADE,
    due_at          timestamptz NOT NULL,
    observation_id  uuid REFERENCES observations (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reminder_completion_occurrence UNIQUE (reminder_id, due_at)
);

CREATE INDEX reminder_completions_reminder ON reminder_completions (reminder_id, due_at DESC);

-- +goose Down

DROP TABLE reminder_completions;
