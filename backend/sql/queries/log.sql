-- name: GetLogHistory :many
SELECT * FROM log_entries
WHERE user_id = ?
ORDER BY date DESC, id;

-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, ingredient_id, date, ingredient_name, ingredient_unit,
   calories_per_unit, protein_per_unit,
   quantity, calories, protein,
   recipe_group_id, source_recipe_name, source_recipe_servings)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLogEntry :exec
DELETE FROM log_entries WHERE id = ? AND user_id = ?;

-- name: DeleteLogEntriesByGroup :exec
DELETE FROM log_entries
WHERE user_id = ?1
  AND date    = ?2
  AND recipe_group_id = ?3;

-- name: SumNutritionByDateRange :many
SELECT
  date,
  CAST(COALESCE(SUM(calories), 0) AS REAL) AS total_calories,
  CAST(COALESCE(SUM(protein),  0) AS REAL) AS total_protein
FROM log_entries
WHERE user_id = sqlc.arg(user_id)
  AND date >= sqlc.arg(from_date)
  AND date <= sqlc.arg(to_date)
GROUP BY date
ORDER BY date;
