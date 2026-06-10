-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(150) UNIQUE NOT NULL,
    password        VARCHAR(255) NOT NULL,
    email           VARCHAR(254) NOT NULL DEFAULT '',
    first_name      VARCHAR(150) NOT NULL DEFAULT '',
    last_name       VARCHAR(150) NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    is_staff        BOOLEAN NOT NULL DEFAULT FALSE,
    is_superuser    BOOLEAN NOT NULL DEFAULT FALSE,
    role            VARCHAR(20) NOT NULL DEFAULT 'operator' CHECK (role IN ('admin','operator')),
    date_joined     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login      TIMESTAMPTZ
);
CREATE INDEX users_username_idx ON users (username);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
