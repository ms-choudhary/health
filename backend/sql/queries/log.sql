-- name: GetLogForDate :many
SELECT * FROM log_entries
WHERE user_id = ? AND date = ?
ORDER BY id;

-- name: GetRecentLoggedFoods :many
SELECT
  le.food_id,
  le.food_name,
  le.food_unit,
  le.calories_per_unit,
  le.protein_per_unit,
  le.quantity AS last_quantity,
  latest.max_id AS max_id
FROM log_entries le
INNER JOIN (
  SELECT inner_le.food_id AS fid, CAST(MAX(inner_le.id) AS INTEGER) AS max_id
  FROM log_entries inner_le
  WHERE inner_le.user_id          = sqlc.arg(user_id)
    AND inner_le.source_recipe_id IS NULL
    AND inner_le.food_id          IS NOT NULL
    AND inner_le.date             >= sqlc.arg(date_floor)
  GROUP BY inner_le.food_id
) latest ON le.id = latest.max_id
ORDER BY le.id DESC
LIMIT 20;

-- name: GetRecentLoggedRecipes :many
SELECT
  r.id   AS recipe_id,
  r.name AS recipe_name,
  COALESCE(le.source_recipe_servings, 1) AS last_servings,
  CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories,
  CAST(COALESCE(SUM(f.protein_per_unit  * ri.quantity), 0) AS REAL) AS total_protein,
  latest.max_id AS max_id
FROM (
  SELECT inner_le.source_recipe_id AS rid, CAST(MAX(inner_le.id) AS INTEGER) AS max_id
  FROM log_entries inner_le
  WHERE inner_le.user_id          = sqlc.arg(user_id)
    AND inner_le.source_recipe_id IS NOT NULL
    AND inner_le.date             >= sqlc.arg(date_floor)
  GROUP BY inner_le.source_recipe_id
) latest
JOIN log_entries le ON le.id = latest.max_id
JOIN recipes      r  ON r.id = latest.rid
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN foods              f  ON f.id          = ri.food_id
GROUP BY r.id, le.source_recipe_servings, latest.max_id
ORDER BY latest.max_id DESC
LIMIT 10;

-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, food_id, date, food_name, food_unit,
   calories_per_unit, protein_per_unit,
   quantity, calories, protein,
   source_recipe_id, source_recipe_name, source_recipe_servings)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLogEntry :exec
DELETE FROM log_entries WHERE id = ? AND user_id = ?;

-- name: DeleteLogEntriesByRecipe :exec
DELETE FROM log_entries
WHERE user_id = ?1
  AND date    = ?2
  AND source_recipe_id = ?3;

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
