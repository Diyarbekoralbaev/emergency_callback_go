-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY username;

-- name: CreateUser :one
INSERT INTO users (username, password, email, first_name, last_name, is_active, is_staff, is_superuser, role)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateUserLastLogin :exec
UPDATE users SET last_login = NOW() WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password = $2 WHERE id = $1;
