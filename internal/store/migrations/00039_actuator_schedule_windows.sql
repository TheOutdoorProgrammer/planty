-- +goose Up

CREATE TABLE plant_actuator_schedule_windows (
    actuator_id  uuid NOT NULL REFERENCES plant_actuator_schedules (actuator_id) ON DELETE CASCADE,
    position     smallint NOT NULL CHECK (position BETWEEN 0 AND 11),
    start_minute int NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute   int NOT NULL CHECK (end_minute BETWEEN 0 AND 1439),
    PRIMARY KEY (actuator_id, position),
    CONSTRAINT plant_actuator_schedule_window_has_duration CHECK (start_minute <> end_minute)
);

INSERT INTO plant_actuator_schedule_windows (actuator_id, position, start_minute, end_minute)
SELECT actuator_id, 0, start_minute, end_minute
FROM plant_actuator_schedules;

-- +goose Down

DROP TABLE plant_actuator_schedule_windows;
