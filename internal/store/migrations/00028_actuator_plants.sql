-- +goose Up

CREATE TABLE plant_actuator_plants (
    actuator_id uuid NOT NULL REFERENCES plant_actuators (id) ON DELETE CASCADE,
    plant_id    uuid NOT NULL REFERENCES plants (id),
    PRIMARY KEY (actuator_id, plant_id)
);

CREATE INDEX plant_actuator_plants_by_plant ON plant_actuator_plants (plant_id, actuator_id);

-- +goose Down

DROP TABLE plant_actuator_plants;
