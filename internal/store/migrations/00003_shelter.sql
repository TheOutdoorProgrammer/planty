-- +goose Up

-- Tracks that a plant was carried indoors ahead of a cold night.
--
-- Without this, the system only ever tells you to bring plants in. Five
-- tropicals left in a dark room for a week is its own way of killing them, and
-- nobody notices because bringing them in already felt like the responsible act.
ALTER TABLE plants ADD COLUMN sheltered_at timestamptz;

CREATE INDEX plants_sheltered ON plants (sheltered_at)
    WHERE archived_at IS NULL AND sheltered_at IS NOT NULL;

-- +goose Down

DROP INDEX plants_sheltered;
ALTER TABLE plants DROP COLUMN sheltered_at;
