-- +goose Up
ALTER TABLE readings ADD COLUMN reported_at timestamptz;
ALTER TABLE sensor_links ADD COLUMN assigned_at timestamptz NOT NULL DEFAULT now();

-- +goose StatementBegin
CREATE FUNCTION reset_sensor_assignment_time() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.plant_id IS DISTINCT FROM OLD.plant_id OR NEW.zone IS DISTINCT FROM OLD.zone THEN
        NEW.assigned_at = now();
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER sensor_assignment_time BEFORE UPDATE ON sensor_links
FOR EACH ROW EXECUTE FUNCTION reset_sensor_assignment_time();

CREATE TABLE watering_detections (
    observation_id uuid PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
    verdict_id uuid NOT NULL REFERENCES verdicts(id),
    sensor_link_id uuid NOT NULL REFERENCES sensor_links(id),
    baseline_reading_id uuid NOT NULL REFERENCES readings(id),
    rise_reading_id uuid NOT NULL REFERENCES readings(id),
    confirmation_reading_id uuid NOT NULL REFERENCES readings(id),
    dry_baseline numeric NOT NULL,
    wet_baseline numeric NOT NULL,
    detected_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE watering_detections;
DROP TRIGGER sensor_assignment_time ON sensor_links;
DROP FUNCTION reset_sensor_assignment_time();
ALTER TABLE sensor_links DROP COLUMN assigned_at;
ALTER TABLE readings DROP COLUMN reported_at;
