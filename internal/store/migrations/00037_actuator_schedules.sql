-- +goose Up

ALTER TABLE plant_light_schedules RENAME TO plant_actuator_schedules;
ALTER TABLE plant_actuator_schedules RENAME CONSTRAINT plant_light_schedule_has_duration TO plant_actuator_schedule_has_duration;

-- +goose Down

ALTER TABLE plant_actuator_schedules RENAME CONSTRAINT plant_actuator_schedule_has_duration TO plant_light_schedule_has_duration;
ALTER TABLE plant_actuator_schedules RENAME TO plant_light_schedules;
