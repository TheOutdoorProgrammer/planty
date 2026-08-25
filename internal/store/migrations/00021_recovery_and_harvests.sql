-- +goose Up

ALTER TABLE photos ADD COLUMN deletion_requested_at timestamptz;

DROP INDEX photos_one_per_plant;
DROP INDEX photos_one_unowned;

CREATE UNIQUE INDEX photos_one_per_plant
    ON photos (plant_id, content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NOT NULL
      AND deletion_requested_at IS NULL;

CREATE UNIQUE INDEX photos_one_unowned
    ON photos (content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NULL
      AND deletion_requested_at IS NULL;

CREATE INDEX photos_pending_deletion
    ON photos (deletion_requested_at, created_at)
    WHERE deletion_requested_at IS NOT NULL OR plant_id IS NULL;

ALTER TABLE harvests ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE harvests ADD CONSTRAINT harvest_quantity_positive CHECK (quantity > 0);
ALTER TABLE harvests ADD CONSTRAINT harvest_unit_present CHECK (length(trim(unit)) > 0);

-- +goose Down

ALTER TABLE harvests DROP CONSTRAINT harvest_unit_present;
ALTER TABLE harvests DROP CONSTRAINT harvest_quantity_positive;
ALTER TABLE harvests DROP COLUMN updated_at;

DROP INDEX photos_pending_deletion;
DROP INDEX photos_one_unowned;
DROP INDEX photos_one_per_plant;

CREATE UNIQUE INDEX photos_one_per_plant
    ON photos (plant_id, content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NOT NULL;

CREATE UNIQUE INDEX photos_one_unowned
    ON photos (content_hash)
    WHERE content_hash IS NOT NULL AND plant_id IS NULL;

ALTER TABLE photos DROP COLUMN deletion_requested_at;
