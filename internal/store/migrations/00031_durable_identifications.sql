-- +goose Up

DROP INDEX conversation_work;

CREATE INDEX model_work
    ON diagnosis_turns (created_at)
    WHERE kind IN ('consult', 'identify') AND status IN ('pending', 'processing');

-- +goose Down

DROP INDEX model_work;

CREATE INDEX conversation_work
    ON diagnosis_turns (created_at)
    WHERE kind = 'consult' AND status IN ('pending', 'processing');
