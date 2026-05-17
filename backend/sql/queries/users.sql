-- name: ListUsers :many
SELECT * FROM users ORDER BY id;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (name, avatar) VALUES (?, ?) RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;
