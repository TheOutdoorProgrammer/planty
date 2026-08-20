-- +goose Up

CREATE TABLE push_devices (
    token       text        NOT NULL,
    environment text        NOT NULL CHECK (environment IN ('sandbox', 'production')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (environment, token)
);

CREATE INDEX push_devices_environment ON push_devices (environment);

-- +goose Down

DROP TABLE push_devices;
