-- +goose Up

ALTER TYPE observation_kind ADD VALUE IF NOT EXISTS 'airflow';

-- +goose Down

-- PostgreSQL cannot remove an enum value without rebuilding every dependent column.
