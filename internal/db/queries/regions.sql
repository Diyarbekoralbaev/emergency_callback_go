-- name: GetRegion :one
SELECT * FROM teams_region WHERE id = $1;

-- name: ListRegions :many
SELECT * FROM teams_region ORDER BY name;

-- name: ListActiveRegions :many
SELECT * FROM teams_region WHERE is_active = TRUE ORDER BY name;

-- name: CreateRegion :one
INSERT INTO teams_region (name, code, description, is_active, created_by_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateRegion :one
UPDATE teams_region
SET name = $2, code = $3, description = $4, is_active = $5
WHERE id = $1
RETURNING *;

-- name: DeleteRegion :exec
DELETE FROM teams_region WHERE id = $1;

-- name: RegionTeamCount :one
SELECT COUNT(*) FROM teams_team WHERE region_id = $1 AND is_active = TRUE;

-- name: RegionTotalTeamCount :one
SELECT COUNT(*) FROM teams_team WHERE region_id = $1;
