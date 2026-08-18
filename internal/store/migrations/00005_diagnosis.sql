-- +goose Up

-- Diagnosis is a conversation, not a one-shot reading: the useful follow-up is
-- "could this be pests?" after the first answer, and that needs the earlier
-- turns to still exist.
CREATE TABLE diagnosis_turns (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id        uuid        NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    conversation_id uuid        NOT NULL,
    asked           text        NOT NULL,
    reply           jsonb       NOT NULL,
    photo_id        uuid REFERENCES photos (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX diagnosis_conversation ON diagnosis_turns (conversation_id, created_at);
CREATE INDEX diagnosis_plant ON diagnosis_turns (plant_id, created_at DESC);

-- +goose Down

DROP TABLE diagnosis_turns;
