-- +goose Up
-- gen_random_uuid() is in Postgres core (13+), so no extension is required.
-- Add shared, app-wide schema objects here as your project grows.
SELECT 1;

-- +goose Down
SELECT 1;
