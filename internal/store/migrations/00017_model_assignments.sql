-- +goose Up

-- One row per job that has been moved off its default. An empty table means
-- every job still answers the way the environment says, so this is additive.
CREATE TABLE model_assignments (
    job        text        NOT NULL PRIMARY KEY,
    provider   text        NOT NULL,
    model      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE model_assignments;
