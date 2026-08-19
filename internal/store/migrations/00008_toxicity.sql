-- +goose Up

-- What a plant does to whatever eats it.
--
-- The record is one JSONB document because most of it is prose nobody filters
-- on, but the three per-audience ratings ARE filtered on: "what in this house
-- can hurt the cat" is the question the feature exists to answer. Those three
-- are generated from the document rather than stored beside it, so a rating
-- and the reasoning behind it cannot drift apart.
ALTER TABLE plants ADD COLUMN toxicity jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE plants
    ADD COLUMN toxic_cats text
        GENERATED ALWAYS AS (coalesce(toxicity ->> 'cats', 'unknown')) STORED,
    ADD COLUMN toxic_dogs text
        GENERATED ALWAYS AS (coalesce(toxicity ->> 'dogs', 'unknown')) STORED,
    ADD COLUMN toxic_people text
        GENERATED ALWAYS AS (coalesce(toxicity ->> 'people', 'unknown')) STORED;

-- Rejects a rating the application does not recognise. Because the columns are
-- generated, this validates the document on the way in: a typo in the JSON
-- fails the write rather than becoming a silent fourth state that renders as
-- neither dangerous nor safe.
ALTER TABLE plants ADD CONSTRAINT plants_toxicity_ratings CHECK (
    toxic_cats   IN ('unknown', 'safe', 'mild', 'moderate', 'severe') AND
    toxic_dogs   IN ('unknown', 'safe', 'mild', 'moderate', 'severe') AND
    toxic_people IN ('unknown', 'safe', 'mild', 'moderate', 'severe')
);

-- Partial, because the overwhelmingly common query is "show me the dangerous
-- ones" and indexing the safe majority buys nothing.
CREATE INDEX plants_dangerous ON plants (toxic_cats, toxic_dogs, toxic_people)
    WHERE archived_at IS NULL
      AND (toxic_cats   IN ('moderate', 'severe')
        OR toxic_dogs   IN ('moderate', 'severe')
        OR toxic_people IN ('moderate', 'severe'));

-- Every existing plant becomes "nobody has checked", never "safe". Defaulting
-- the other way would turn an empty column into a house full of reassurance
-- that nothing has earned.
CREATE INDEX plants_toxicity_unchecked ON plants (id)
    WHERE archived_at IS NULL AND toxicity = '{}'::jsonb;

-- +goose Down

DROP INDEX plants_toxicity_unchecked;
DROP INDEX plants_dangerous;
ALTER TABLE plants DROP CONSTRAINT plants_toxicity_ratings;
ALTER TABLE plants
    DROP COLUMN toxic_people,
    DROP COLUMN toxic_dogs,
    DROP COLUMN toxic_cats,
    DROP COLUMN toxicity;
