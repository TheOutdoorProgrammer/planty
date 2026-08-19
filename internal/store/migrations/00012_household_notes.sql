-- +goose Up

-- Notes about the house rather than about a plant.
--
-- "There is a cat here" is not a fact about the pothos, but it changes the
-- advice for every plant in the place. With nowhere to put it, the model kept
-- writing it against whichever plant happened to be under discussion, and then
-- went hunting for a memory file to write it to instead.
--
-- Renamed while the table is one migration old: it stopped being about plants.
ALTER TABLE plant_notes RENAME TO notes;
ALTER TABLE notes ALTER COLUMN plant_id DROP NOT NULL;

ALTER INDEX plant_notes_pkey RENAME TO notes_pkey;
ALTER INDEX plant_notes_plant RENAME TO notes_plant;

-- The household's own notes are read on every consultation, so they get their
-- own index rather than a scan past every plant note in the database.
CREATE INDEX notes_household ON notes (created_at DESC) WHERE plant_id IS NULL;

-- +goose Down

DROP INDEX notes_household;

DELETE FROM notes WHERE plant_id IS NULL;
ALTER TABLE notes ALTER COLUMN plant_id SET NOT NULL;

ALTER INDEX notes_plant RENAME TO plant_notes_plant;
ALTER INDEX notes_pkey RENAME TO plant_notes_pkey;
ALTER TABLE notes RENAME TO plant_notes;
