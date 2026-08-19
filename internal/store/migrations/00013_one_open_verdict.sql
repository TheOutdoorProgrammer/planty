-- +goose Up

-- A new judgment supersedes an older instruction for the same plant.
-- Keep the newest open row and close any historical rows left by the old model.
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY plant_id
               ORDER BY for_date DESC, created_at DESC, id DESC
           ) AS position
    FROM verdicts
    WHERE acknowledged_at IS NULL
)
UPDATE verdicts AS verdict
SET acknowledged_at = now()
FROM ranked
WHERE verdict.id = ranked.id
  AND ranked.position > 1;

CREATE UNIQUE INDEX verdicts_one_open_per_plant
    ON verdicts (plant_id)
    WHERE acknowledged_at IS NULL;

-- +goose Down

DROP INDEX verdicts_one_open_per_plant;
