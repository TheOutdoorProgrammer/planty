-- +goose Up

-- Aggregate counts say whether a run was incomplete, but not which plant was
-- missed or what the model returned. Per-plant rows make a partial run
-- diagnosable and let an operator retry only failures without touching a
-- successful verdict.
CREATE TABLE judgment_results (
    run_id          uuid NOT NULL REFERENCES judgment_runs (id) ON DELETE CASCADE,
    plant_id        uuid NOT NULL REFERENCES plants (id),
    succeeded       boolean NOT NULL,
    attempts        int NOT NULL DEFAULT 1 CHECK (attempts > 0),
    model            text NOT NULL DEFAULT '',
    original_error   text NOT NULL DEFAULT '',
    original_output  text NOT NULL DEFAULT '',
    final_error      text NOT NULL DEFAULT '',
    updated_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, plant_id)
);

CREATE INDEX judgment_results_failed
    ON judgment_results (run_id, plant_id)
    WHERE succeeded = false;

-- +goose Down

DROP TABLE judgment_results;
