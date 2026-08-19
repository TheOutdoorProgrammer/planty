-- +goose Up

-- Notes a person keeps about a plant, in their own words.
--
-- Separate from care_profile.notes, which is one field the whole app
-- overwrites: these are many, each editable and deletable on its own, so
-- "repotted it in March" and "the cat keeps chewing this one" stop competing
-- for the same string.
CREATE TABLE plant_notes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id   uuid        NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    title      text,
    body       text        NOT NULL CHECK (body <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX plant_notes_plant ON plant_notes (plant_id, created_at DESC);

-- +goose Down

DROP TABLE plant_notes;
