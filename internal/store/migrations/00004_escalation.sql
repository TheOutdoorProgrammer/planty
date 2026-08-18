-- +goose Up

-- How many times this verdict has been chased.
--
-- Bounded rather than open-ended: a notification that repeats forever trains
-- you to swipe it away, and then the channel is worth nothing for the plant
-- that actually needs you.
ALTER TABLE verdicts ADD COLUMN escalations int NOT NULL DEFAULT 0;
ALTER TABLE verdicts ADD COLUMN escalated_at timestamptz;

-- The escalation sweep asks only this question.
CREATE INDEX verdicts_chaseable ON verdicts (for_date, escalations)
    WHERE acknowledged_at IS NULL AND action <> 'none';

-- +goose Down

DROP INDEX verdicts_chaseable;
ALTER TABLE verdicts DROP COLUMN escalated_at;
ALTER TABLE verdicts DROP COLUMN escalations;
