-- +goose Up

-- A daily judgment is one garden-wide claim. Persisting the run separately
-- from individual verdicts means a partial model outage can never look like a
-- complete, fresh all-clear merely because one plant succeeded.
CREATE TABLE judgment_runs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    started_at   timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    expected     int NOT NULL CHECK (expected >= 0),
    succeeded    int NOT NULL DEFAULT 0 CHECK (succeeded >= 0),
    failed       int NOT NULL DEFAULT 0 CHECK (failed >= 0),
    CONSTRAINT judgment_run_counts_fit CHECK (succeeded + failed <= expected)
);

CREATE INDEX judgment_runs_started ON judgment_runs (started_at DESC);

-- Completing a verdict is one transaction and one idempotency key. A client
-- may safely retry after losing the HTTP response without writing the same care
-- observation twice or leaving the verdict open.
CREATE TABLE care_completions (
    idempotency_key uuid PRIMARY KEY,
    verdict_id      uuid NOT NULL REFERENCES verdicts (id),
    kind            observation_kind NOT NULL,
    body            text NOT NULL DEFAULT '',
    observation_id  uuid REFERENCES observations (id),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX care_completions_verdict ON care_completions (verdict_id);

-- +goose Down

DROP TABLE care_completions;
DROP TABLE judgment_runs;
