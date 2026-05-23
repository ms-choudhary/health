-- name: ListUsers :many
SELECT * FROM users ORDER BY id;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (name, avatar, target_calories, target_protein)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name            = ?,
    target_calories = ?,
    target_protein  = ?
WHERE id = ?
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;
