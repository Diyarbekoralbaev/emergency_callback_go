-- +goose Up
-- +goose StatementBegin
CREATE TABLE teams_region (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) UNIQUE NOT NULL,
    code            VARCHAR(20) UNIQUE NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX teams_region_name_idx ON teams_region (name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS teams_region;
-- +goose StatementEnd
