-- name: GetLogForDate :many
SELECT * FROM log_entries
WHERE user_id = ? AND date = ?
ORDER BY id;

-- name: GetRecentLoggedFoods :many
SELECT
  le.food_name,
  le.food_unit,
  le.calories_per_unit,
  le.food_id,
  le.quantity AS last_quantity
FROM log_entries le
INNER JOIN (
  SELECT inner_le.food_name AS fn, MAX(inner_le.id) AS max_id
  FROM log_entries inner_le
  WHERE inner_le.user_id = ?1
    AND inner_le.source_recipe_id IS NULL
  GROUP BY inner_le.food_name
) latest ON le.id = latest.max_id
ORDER BY le.id DESC
LIMIT 20;

-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, food_id, date, food_name, food_unit, calories_per_unit, quantity, calories,
   source_recipe_id, source_recipe_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLogEntry :exec
DELETE FROM log_entries WHERE id = ? AND user_id = ?;

-- name: DeleteLogEntriesByRecipe :exec
DELETE FROM log_entries
WHERE user_id = ?1
  AND date    = ?2
  AND source_recipe_id = ?3;

-- name: SumCaloriesByDateRange :many
SELECT
  date,
  CAST(COALESCE(SUM(calories), 0) AS REAL) AS total_calories
FROM log_entries
WHERE user_id = sqlc.arg(user_id)
  AND date >= sqlc.arg(from_date)
  AND date <= sqlc.arg(to_date)
GROUP BY date
ORDER BY date;
