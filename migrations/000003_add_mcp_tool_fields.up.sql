-- +goose Up
ALTER TABLE tools
ADD COLUMN endpoint_url TEXT,
ADD COLUMN auth_token TEXT,
ADD COLUMN connection_settings JSONB;

-- +goose Down
ALTER TABLE tools
DROP COLUMN endpoint_url,
DROP COLUMN auth_token,
DROP COLUMN connection_settings;