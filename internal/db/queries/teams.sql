-- name: GetTeam :one
SELECT * FROM teams_team WHERE id = $1;

-- name: GetTeamWithRegion :one
SELECT t.*, r.name AS region_name, r.code AS region_code
FROM teams_team t
JOIN teams_region r ON r.id = t.region_id
WHERE t.id = $1;

-- name: ListTeams :many
SELECT t.*, r.name AS region_name, r.code AS region_code
FROM teams_team t
JOIN teams_region r ON r.id = t.region_id
ORDER BY r.name, t.name;

-- name: ListActiveTeams :many
SELECT t.*, r.name AS region_name, r.code AS region_code
FROM teams_team t
JOIN teams_region r ON r.id = t.region_id
WHERE t.is_active = TRUE
ORDER BY r.name, t.name;

-- name: ListTeamsByRegion :many
SELECT t.*, r.name AS region_name
FROM teams_team t
JOIN teams_region r ON r.id = t.region_id
WHERE t.region_id = $1 AND t.is_active = TRUE
ORDER BY t.name;

-- name: CreateTeam :one
INSERT INTO teams_team (name, description, region_id, is_active, created_by_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateTeam :one
UPDATE teams_team
SET name = $2, description = $3, region_id = $4, is_active = $5
WHERE id = $1
RETURNING *;

-- name: DeleteTeam :exec
DELETE FROM teams_team WHERE id = $1;

-- name: RandomActiveTeam :one
SELECT * FROM teams_team WHERE is_active = TRUE ORDER BY random() LIMIT 1;
