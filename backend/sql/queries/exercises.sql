-- name: ListExercises :many
SELECT * FROM exercises
WHERE name LIKE '%' || sqlc.arg(search) || '%'
ORDER BY name;

-- name: GetExercise :one
SELECT * FROM exercises WHERE id = ?;

-- name: CreateExercise :one
INSERT INTO exercises (name, notes) VALUES (?, ?) RETURNING *;

-- name: UpdateExercise :one
UPDATE exercises SET name = ?, notes = ? WHERE id = ? RETURNING *;

-- name: DeleteExercise :exec
DELETE FROM exercises WHERE id = ?;
