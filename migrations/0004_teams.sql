-- +goose Up
-- +goose StatementBegin
CREATE TABLE teams_team (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    region_id       BIGINT NOT NULL REFERENCES teams_region(id) ON DELETE CASCADE,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (name, region_id)
);
CREATE INDEX teams_team_region_idx ON teams_team (region_id);
CREATE INDEX teams_team_active_idx ON teams_team (is_active);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS teams_team;
-- +goose StatementEnd
