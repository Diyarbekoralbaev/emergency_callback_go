-- +goose Up
-- +goose StatementBegin
CREATE TABLE callbacks_rating (
    id                      BIGSERIAL PRIMARY KEY,
    callback_request_id     BIGINT UNIQUE NOT NULL REFERENCES callbacks_callbackrequest(id) ON DELETE CASCADE,
    rating                  INTEGER NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment                 TEXT,
    timestamp               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    phone_number            VARCHAR(20) NOT NULL,
    team_id                 BIGINT NOT NULL REFERENCES teams_team(id) ON DELETE CASCADE,
    date                    DATE NOT NULL DEFAULT CURRENT_DATE
);
CREATE INDEX callbacks_rating_rating_idx ON callbacks_rating (rating);
CREATE INDEX callbacks_rating_date_idx ON callbacks_rating (date);
CREATE INDEX callbacks_rating_team_idx ON callbacks_rating (team_id);
CREATE INDEX callbacks_rating_timestamp_idx ON callbacks_rating (timestamp);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS callbacks_rating;
-- +goose StatementEnd
