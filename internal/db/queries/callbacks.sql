-- name: GetCallback :one
SELECT * FROM callbacks_callbackrequest WHERE id = $1;

-- name: GetCallbackByVoteUUID :one
SELECT * FROM callbacks_callbackrequest WHERE vote_uuid = $1;

-- name: GetCallbackByCallID :one
SELECT * FROM callbacks_callbackrequest WHERE call_id = $1;

-- name: CreateCallback :one
INSERT INTO callbacks_callbackrequest (phone_number, team_id, status, requested_by_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateCallbackStatus :exec
UPDATE callbacks_callbackrequest
SET status = $2, call_started_at = COALESCE($3, call_started_at)
WHERE id = $1;

-- name: UpdateCallbackResult :exec
UPDATE callbacks_callbackrequest
SET status = $2,
    call_id = COALESCE($3, call_id),
    call_ended_at = $4,
    call_duration = $5,
    transferred = $6,
    error_message = $7
WHERE id = $1;

-- name: UpdateCallbackSMSSent :exec
UPDATE callbacks_callbackrequest
SET sms_sent = TRUE, sms_sent_at = NOW(), status = $2
WHERE id = $1;

-- name: UpdateCallbackVotedViaSMS :exec
UPDATE callbacks_callbackrequest
SET voted_via_sms = TRUE, status = 'completed'
WHERE id = $1;

-- name: ListCallbacks :many
SELECT * FROM callbacks_callbackrequest
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountCallbacks :one
SELECT COUNT(*) FROM callbacks_callbackrequest;

-- name: ListCallbacksByTeam :many
SELECT * FROM callbacks_callbackrequest
WHERE team_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: FindStaleCallbacks :many
SELECT * FROM callbacks_callbackrequest
WHERE status IN ('dialing','connecting','answered','waiting_rating','waiting_additional','transferring')
  AND call_started_at IS NOT NULL
  AND call_started_at < $1;

-- name: CallbackHasRating :one
SELECT EXISTS(SELECT 1 FROM callbacks_rating WHERE callback_request_id = $1) AS has_rating;
