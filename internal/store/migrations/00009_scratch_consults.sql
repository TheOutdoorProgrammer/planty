-- +goose Up

-- A question about a plant you do not own.
--
-- Pointing the camera at something in a shop and asking whether it would kill
-- the cat is a real question, and the answer is usually "do not buy it", so
-- forcing it to create a plant record first files a row for a plant that was
-- never brought home. Both tables lose their NOT NULL rather than gaining a
-- parallel scratch table, because every existing query filters on a plant id
-- and `plant_id = $1` already declines to match a null.
ALTER TABLE photos ALTER COLUMN plant_id DROP NOT NULL;
ALTER TABLE diagnosis_turns ALTER COLUMN plant_id DROP NOT NULL;

-- Scratch photographs belong to nothing, so nothing would ever delete them.
-- The conversation they were taken for is the only owner they have.
CREATE INDEX photos_unowned ON photos (created_at) WHERE plant_id IS NULL;

-- +goose Down

DROP INDEX photos_unowned;

DELETE FROM diagnosis_turns WHERE plant_id IS NULL;
DELETE FROM photos WHERE plant_id IS NULL;

ALTER TABLE diagnosis_turns ALTER COLUMN plant_id SET NOT NULL;
ALTER TABLE photos ALTER COLUMN plant_id SET NOT NULL;
