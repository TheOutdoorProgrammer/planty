-- +goose Up

CREATE TYPE garden_incident_status AS ENUM ('open', 'acknowledged', 'resolved');
CREATE TYPE garden_incident_factor AS ENUM ('ha_area', 'location', 'common_care', 'environment_failure', 'actuator_failure');
CREATE TYPE garden_incident_resolution AS ENUM ('confirmed_common_cause', 'unrelated', 'contained', 'inconclusive');

CREATE TABLE garden_incidents (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status                garden_incident_status NOT NULL DEFAULT 'open',
    suspected_factor_type garden_incident_factor NOT NULL,
    suspected_factor_ref  text NOT NULL CHECK (length(btrim(suspected_factor_ref)) > 0),
    summary               text NOT NULL CHECK (length(btrim(summary)) > 0),
    confidence            numeric NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    evidence              jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    detected_run_id       uuid NOT NULL REFERENCES judgment_runs (id),
    first_seen_at         timestamptz NOT NULL DEFAULT now(),
    last_seen_at          timestamptz NOT NULL DEFAULT now(),
    acknowledged_at       timestamptz,
    acknowledged_by       text NOT NULL DEFAULT '',
    resolved_at           timestamptz,
    resolved_by           text NOT NULL DEFAULT '',
    resolution            garden_incident_resolution,
    conclusion            text NOT NULL DEFAULT '',
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT garden_incident_resolution_shape CHECK (
        (status <> 'resolved' AND resolved_at IS NULL AND resolution IS NULL) OR
        (status = 'resolved' AND resolved_at IS NOT NULL AND resolution IS NOT NULL AND length(btrim(resolved_by)) > 0)
    )
);

CREATE INDEX garden_incidents_status_updated ON garden_incidents (status, updated_at DESC);
CREATE UNIQUE INDEX garden_incidents_open_factor
    ON garden_incidents (suspected_factor_type, suspected_factor_ref)
    WHERE status <> 'resolved';

CREATE TABLE garden_incident_detections (
    incident_id uuid NOT NULL REFERENCES garden_incidents (id) ON DELETE CASCADE,
    run_id      uuid NOT NULL REFERENCES judgment_runs (id),
    confidence  numeric NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    evidence    jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    detected_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, run_id)
);

CREATE TABLE garden_incident_plants (
    incident_id   uuid NOT NULL REFERENCES garden_incidents (id) ON DELETE CASCADE,
    plant_id      uuid NOT NULL REFERENCES plants (id),
    role          text NOT NULL DEFAULT 'affected' CHECK (role IN ('affected', 'exposed')),
    evidence      jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (incident_id, plant_id)
);

CREATE INDEX garden_incident_plants_by_plant ON garden_incident_plants (plant_id, incident_id);

-- +goose Down

DROP TABLE garden_incident_plants;
DROP TABLE garden_incident_detections;
DROP TABLE garden_incidents;
DROP TYPE garden_incident_resolution;
DROP TYPE garden_incident_factor;
DROP TYPE garden_incident_status;
