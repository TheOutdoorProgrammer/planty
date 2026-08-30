-- +goose Up

ALTER TYPE plant_actuator_kind ADD VALUE IF NOT EXISTS 'water';

ALTER TABLE plant_actuators DROP CONSTRAINT actuator_entity_matches_kind;
ALTER TABLE plant_actuators ADD CONSTRAINT actuator_entity_domain_allowed CHECK (
    entity_id ~ '^(fan|switch|light)[.][a-z0-9_]+$'
);

-- +goose Down

ALTER TABLE plant_actuators DROP CONSTRAINT actuator_entity_domain_allowed;
ALTER TABLE plant_actuators ADD CONSTRAINT actuator_entity_matches_kind CHECK (
    entity_id ~ '^(fan|switch|light)[.][a-z0-9_]+$'
);
