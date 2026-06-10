-- +goose Up
-- +goose StatementBegin
CREATE TABLE callbacks_callbackrequest (
    id                      BIGSERIAL PRIMARY KEY,
    phone_number            VARCHAR(20) NOT NULL,
    team_id                 BIGINT NOT NULL REFERENCES teams_team(id) ON DELETE CASCADE,
    status                  VARCHAR(20) NOT NULL DEFAULT 'pending',
    call_id                 UUID UNIQUE DEFAULT gen_random_uuid(),
    uniqueid                VARCHAR(100),
    channel                 VARCHAR(100),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    call_started_at         TIMESTAMPTZ,
    call_ended_at           TIMESTAMPTZ,
    call_duration           INTEGER,
    error_message           TEXT,
    transferred             BOOLEAN NOT NULL DEFAULT FALSE,
    additional_questions    BOOLEAN,
    requested_by_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vote_uuid               UUID UNIQUE NOT NULL DEFAULT gen_random_uuid(),
    sms_sent                BOOLEAN NOT NULL DEFAULT FALSE,
    sms_sent_at             TIMESTAMPTZ,
    voted_via_sms           BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX callbacks_callbackrequest_status_idx ON callbacks_callbackrequest (status);
CREATE INDEX callbacks_callbackrequest_created_at_idx ON callbacks_callbackrequest (created_at);
CREATE INDEX callbacks_callbackrequest_phone_idx ON callbacks_callbackrequest (phone_number);
CREATE INDEX callbacks_callbackrequest_team_idx ON callbacks_callbackrequest (team_id);
CREATE INDEX callbacks_callbackrequest_vote_uuid_idx ON callbacks_callbackrequest (vote_uuid);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS callbacks_callbackrequest;
-- +goose StatementEnd
