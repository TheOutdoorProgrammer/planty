-- +goose Up

CREATE TYPE evidence_window_kind AS ENUM ('recheck', 'experiment');
CREATE TYPE evidence_window_status AS ENUM ('proposed', 'active', 'ready', 'completed', 'cancelled');
CREATE TYPE evidence_window_outcome AS ENUM (
    'improved', 'unchanged', 'worsened', 'insufficient_evidence',
    'supported', 'not_supported', 'inconclusive', 'stopped_for_safety', 'cancelled'
);
CREATE TYPE evidence_reference_kind AS ENUM ('observation', 'photo', 'reading');
CREATE TYPE evidence_reference_phase AS ENUM ('baseline', 'review');

CREATE TABLE evidence_windows (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                        evidence_window_kind NOT NULL,
    status                      evidence_window_status NOT NULL DEFAULT 'proposed',
    intervention_kind           observation_kind NOT NULL,
    intervention_observation_id uuid REFERENCES observations (id),
    earliest_review_at          timestamptz NOT NULL,
    latest_review_at            timestamptz NOT NULL,
    started_at                  timestamptz,
    ready_at                    timestamptz,
    completed_at                timestamptz,
    outcome                     evidence_window_outcome,
    conclusion                  text NOT NULL DEFAULT '',
    confounded_at               timestamptz,
    confound_reason             text NOT NULL DEFAULT '',
    proposed_by                 observation_source NOT NULL,
    proposed_actor              text NOT NULL DEFAULT '',
    started_by                  observation_source,
    started_actor               text NOT NULL DEFAULT '',
    completed_by                observation_source,
    completed_actor             text NOT NULL DEFAULT '',
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT evidence_window_review_order CHECK (
        latest_review_at > earliest_review_at
        AND latest_review_at <= created_at + interval '90 days'
    ),
    CONSTRAINT evidence_window_state_shape CHECK (
        (status = 'proposed' AND started_at IS NULL AND ready_at IS NULL AND completed_at IS NULL AND outcome IS NULL)
        OR (status = 'active' AND started_at IS NOT NULL AND ready_at IS NULL AND completed_at IS NULL AND outcome IS NULL)
        OR (status = 'ready' AND started_at IS NOT NULL AND ready_at IS NOT NULL AND completed_at IS NULL AND outcome IS NULL)
        OR (status IN ('completed', 'cancelled') AND completed_at IS NOT NULL AND outcome IS NOT NULL)
    )
);

CREATE INDEX evidence_windows_open_review
    ON evidence_windows (latest_review_at)
    WHERE status IN ('active', 'ready');

CREATE TABLE evidence_window_plants (
    window_id uuid NOT NULL REFERENCES evidence_windows (id) ON DELETE CASCADE,
    plant_id  uuid NOT NULL REFERENCES plants (id),
    PRIMARY KEY (window_id, plant_id)
);

CREATE INDEX evidence_window_plants_by_plant ON evidence_window_plants (plant_id, window_id);

CREATE TABLE evidence_window_refs (
    window_id   uuid NOT NULL REFERENCES evidence_windows (id) ON DELETE CASCADE,
    plant_id    uuid NOT NULL REFERENCES plants (id),
    kind        evidence_reference_kind NOT NULL,
    evidence_id uuid NOT NULL,
    phase       evidence_reference_phase NOT NULL,
    PRIMARY KEY (window_id, phase, kind, evidence_id),
    FOREIGN KEY (window_id, plant_id) REFERENCES evidence_window_plants (window_id, plant_id)
);

CREATE TABLE evidence_window_expectations (
    window_id   uuid NOT NULL REFERENCES evidence_windows (id) ON DELETE CASCADE,
    plant_id    uuid NOT NULL REFERENCES plants (id),
    kind        evidence_reference_kind NOT NULL,
    instruction text NOT NULL CHECK (length(btrim(instruction)) > 0),
    PRIMARY KEY (window_id, plant_id, kind),
    FOREIGN KEY (window_id, plant_id) REFERENCES evidence_window_plants (window_id, plant_id)
);

CREATE TABLE evidence_window_guardrails (
    window_id         uuid PRIMARY KEY REFERENCES evidence_windows (id) ON DELETE CASCADE,
    reason            text NOT NULL CHECK (length(btrim(reason)) > 0),
    conflicting_kinds observation_kind[] NOT NULL CHECK (cardinality(conflicting_kinds) > 0),
    red_flags         text[] NOT NULL CHECK (cardinality(red_flags) > 0)
);

CREATE TABLE evidence_window_guardrail_overrides (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    window_id  uuid NOT NULL REFERENCES evidence_windows (id) ON DELETE CASCADE,
    plant_id   uuid NOT NULL REFERENCES plants (id),
    kind       observation_kind NOT NULL,
    reason     text NOT NULL CHECK (length(btrim(reason)) > 0),
    source     observation_source NOT NULL,
    actor      text NOT NULL CHECK (length(btrim(actor)) > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (window_id, plant_id) REFERENCES evidence_window_plants (window_id, plant_id)
);

CREATE TABLE evidence_window_experiments (
    window_id           uuid PRIMARY KEY REFERENCES evidence_windows (id) ON DELETE CASCADE,
    title               text NOT NULL CHECK (length(btrim(title)) > 0),
    hypothesis          text NOT NULL CHECK (length(btrim(hypothesis)) > 0),
    variable_kind       text NOT NULL CHECK (length(btrim(variable_kind)) > 0),
    variable_value      text NOT NULL CHECK (length(btrim(variable_value)) > 0),
    hold_constant_rules text[] NOT NULL CHECK (cardinality(hold_constant_rules) > 0),
    success_criteria    text[] NOT NULL CHECK (cardinality(success_criteria) > 0)
);

-- Recording reality always succeeds through the ordinary observation path.
-- When it conflicts with an active guardrail, the same write atomically marks
-- the evidence window confounded instead of pretending the variable held.
-- +goose StatementBegin
CREATE FUNCTION confound_evidence_windows_from_observation() RETURNS trigger AS $$
BEGIN
    UPDATE evidence_windows w
       SET confounded_at = coalesce(w.confounded_at, NEW.occurred_at),
           confound_reason = CASE WHEN w.confound_reason = ''
               THEN 'conflicting ' || NEW.kind::text || ' observation recorded'
               ELSE w.confound_reason END,
           updated_at = now()
      FROM evidence_window_guardrails g, evidence_window_plants p
     WHERE g.window_id = w.id
       AND p.window_id = w.id
       AND p.plant_id = NEW.plant_id
       AND NEW.kind = ANY(g.conflicting_kinds)
       AND w.status IN ('active', 'ready')
       AND NEW.occurred_at >= w.started_at
       AND NEW.id IS DISTINCT FROM w.intervention_observation_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER observations_confound_evidence_windows
AFTER INSERT ON observations
FOR EACH ROW EXECUTE FUNCTION confound_evidence_windows_from_observation();

-- +goose Down

DROP TRIGGER observations_confound_evidence_windows ON observations;
DROP FUNCTION confound_evidence_windows_from_observation;
DROP TABLE evidence_window_experiments;
DROP TABLE evidence_window_guardrail_overrides;
DROP TABLE evidence_window_guardrails;
DROP TABLE evidence_window_expectations;
DROP TABLE evidence_window_refs;
DROP TABLE evidence_window_plants;
DROP TABLE evidence_windows;
DROP TYPE evidence_reference_phase;
DROP TYPE evidence_reference_kind;
DROP TYPE evidence_window_outcome;
DROP TYPE evidence_window_status;
DROP TYPE evidence_window_kind;
