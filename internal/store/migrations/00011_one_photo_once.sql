-- +goose Up

-- The same picture, filed twice.
--
-- Saving a photograph and then asking about it are two separate uploads of one
-- capture, and a failed save that gets retried is another. Nothing rejected
-- them, so a plant ended up with two rows a person cannot tell apart, both
-- offered to the model and both paid for.
--
-- Bytes are the identity: a camera does not produce the same file twice, so
-- an identical hash is the same upload arriving again.
ALTER TABLE photos ADD COLUMN content_hash text;

-- Nulls do not collide in Postgres, so photographs stored before this keep
-- their rows and only new uploads are held to it.
CREATE UNIQUE INDEX photos_one_per_plant
    ON photos (plant_id, content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NOT NULL;

-- The same picture asked about twice with no plant behind it is still one
-- picture, and those rows are owned by nothing that would clean them up.
CREATE UNIQUE INDEX photos_one_unowned
    ON photos (content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NULL;

-- +goose Down

DROP INDEX photos_one_unowned;
DROP INDEX photos_one_per_plant;
ALTER TABLE photos DROP COLUMN content_hash;
