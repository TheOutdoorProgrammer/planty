-- +goose Up

-- A consultation is a conversation about the record rather than about
-- photographs, and the two must not replay into each other: a diagnosis reply
-- and an answer are different shapes, and mixing them corrupts both.
ALTER TABLE diagnosis_turns
    ADD COLUMN kind text NOT NULL DEFAULT 'diagnosis';

DROP INDEX diagnosis_conversation;
CREATE INDEX conversation_turns ON diagnosis_turns (kind, conversation_id, created_at);

-- +goose Down

DROP INDEX conversation_turns;
CREATE INDEX diagnosis_conversation ON diagnosis_turns (conversation_id, created_at);

ALTER TABLE diagnosis_turns DROP COLUMN kind;
