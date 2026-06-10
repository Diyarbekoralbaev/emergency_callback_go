-- name: GetRatingByCallback :one
SELECT * FROM callbacks_rating WHERE callback_request_id = $1;

-- name: CreateRating :one
INSERT INTO callbacks_rating (callback_request_id, rating, comment, phone_number, team_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListRatings :many
SELECT * FROM callbacks_rating
ORDER BY timestamp DESC
LIMIT $1 OFFSET $2;

-- name: CountRatings :one
SELECT COUNT(*) FROM callbacks_rating;

-- name: AvgRating :one
SELECT COALESCE(AVG(rating), 0)::FLOAT8 AS avg_rating FROM callbacks_rating;

-- name: RatingDistribution :many
SELECT rating, COUNT(*) AS count
FROM callbacks_rating
GROUP BY rating
ORDER BY rating;

-- name: RatingsByTeam :many
SELECT * FROM callbacks_rating WHERE team_id = $1 ORDER BY timestamp DESC;
