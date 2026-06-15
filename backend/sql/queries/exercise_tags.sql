-- name: CreateExerciseTag :one
INSERT INTO exercise_tags (name) VALUES (?) RETURNING *;

-- name: GetExerciseTag :one
SELECT * FROM exercise_tags WHERE id = ?;

-- name: GetExerciseTagByName :one
SELECT * FROM exercise_tags WHERE name = ? COLLATE NOCASE;

-- name: DeleteExerciseTag :exec
DELETE FROM exercise_tags WHERE id = ?;

-- name: ListExerciseTags :many
SELECT
  t.id, t.name, t.created_at,
  CAST(COUNT(et.exercise_id) AS INTEGER) AS exercise_count
FROM exercise_tags t
LEFT JOIN exercise_taggings et ON et.exercise_tag_id = t.id
GROUP BY t.id
ORDER BY t.name COLLATE NOCASE;

-- name: GetTagsForExercise :many
SELECT t.id, t.name, t.created_at
FROM exercise_taggings et
JOIN exercise_tags t ON t.id = et.exercise_tag_id
WHERE et.exercise_id = ?
ORDER BY t.name COLLATE NOCASE;

-- name: ListAllExerciseTagLinks :many
SELECT et.exercise_id, t.id, t.name, t.created_at
FROM exercise_taggings et
JOIN exercise_tags t ON t.id = et.exercise_tag_id
ORDER BY t.name COLLATE NOCASE;

-- name: AddExerciseTag :exec
INSERT OR IGNORE INTO exercise_taggings (exercise_id, exercise_tag_id) VALUES (?, ?);

-- name: ClearExerciseTags :exec
DELETE FROM exercise_taggings WHERE exercise_id = ?;

-- name: ListExercisesByTag :many
SELECT e.id, e.name, e.notes, e.created_at
FROM exercises e
JOIN exercise_taggings et ON et.exercise_id = e.id
WHERE et.exercise_tag_id = ?
ORDER BY e.name;
