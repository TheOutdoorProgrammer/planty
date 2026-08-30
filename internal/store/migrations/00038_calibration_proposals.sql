-- +goose Up

CREATE TABLE sensor_calibration_proposals (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sensor_link_id     uuid NOT NULL REFERENCES sensor_links (id) ON DELETE CASCADE,
    plant_id           uuid NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    reading_id         uuid NOT NULL REFERENCES readings (id),
    actual_value       double precision NOT NULL,
    unit               text NOT NULL DEFAULT '',
    current_dry        double precision NOT NULL,
    current_wet        double precision NOT NULL,
    proposed_dry       double precision NOT NULL,
    proposed_wet       double precision NOT NULL,
    current_relative   double precision NOT NULL CHECK (current_relative BETWEEN 0 AND 1),
    proposed_relative  double precision NOT NULL CHECK (proposed_relative BETWEEN 0 AND 1),
    reason             text NOT NULL CHECK (length(trim(reason)) > 0),
    model_version      text NOT NULL DEFAULT '',
    status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    resolved_at        timestamptz,
    resolved_by        text NOT NULL DEFAULT '',
    CONSTRAINT sensor_calibration_proposal_dry_wet CHECK (proposed_wet > proposed_dry)
);

CREATE UNIQUE INDEX sensor_calibration_one_pending
    ON sensor_calibration_proposals (sensor_link_id) WHERE status = 'pending';
CREATE INDEX sensor_calibration_proposal_plant
    ON sensor_calibration_proposals (plant_id, created_at DESC);

-- +goose Down

DROP TABLE sensor_calibration_proposals;
