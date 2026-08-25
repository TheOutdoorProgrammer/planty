-- +goose Up

ALTER TABLE push_devices
    ADD COLUMN installation_id uuid,
    ADD COLUMN accepted_at timestamptz NOT NULL DEFAULT now();

UPDATE push_devices SET installation_id = gen_random_uuid() WHERE installation_id IS NULL;

ALTER TABLE push_devices ALTER COLUMN installation_id SET NOT NULL;

CREATE UNIQUE INDEX push_devices_installation
    ON push_devices (environment, installation_id);

-- +goose Down

DROP INDEX push_devices_installation;
ALTER TABLE push_devices DROP COLUMN accepted_at;
ALTER TABLE push_devices DROP COLUMN installation_id;
