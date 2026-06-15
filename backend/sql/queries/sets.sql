-- name: AddSet :one
INSERT INTO exercise_sets (user_id, exercise_id, exercise_name, date, weight, reps, unit)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSetHistory :many
SELECT * FROM exercise_sets
WHERE user_id = ?
ORDER BY date DESC, id DESC;

-- name: GetLastSetsForExercise :many
SELECT es.* FROM exercise_sets es
WHERE es.user_id = sqlc.arg(user_id)
  AND es.exercise_id = sqlc.arg(exercise_id)
  AND es.date = (
    SELECT MAX(inner_es.date) FROM exercise_sets inner_es
    WHERE inner_es.user_id = sqlc.arg(user_id) AND inner_es.exercise_id = sqlc.arg(exercise_id)
  )
ORDER BY es.id;

-- name: UpdateSet :one
UPDATE exercise_sets SET weight = ?, reps = ?
WHERE id = ? AND user_id = ?
RETURNING *;

-- name: DeleteSet :exec
DELETE FROM exercise_sets WHERE id = ? AND user_id = ?;

-- name: GetProgressSets :many
SELECT s.exercise_id, s.exercise_name, s.date, s.weight, s.reps
FROM exercise_sets s
WHERE s.user_id = sqlc.arg(user_id)
  AND s.date >= sqlc.arg(from_date)
  AND s.date <= sqlc.arg(to_date)
ORDER BY s.exercise_name COLLATE NOCASE, s.date, s.id;
