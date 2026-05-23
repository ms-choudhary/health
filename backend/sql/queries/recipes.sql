-- name: CreateRecipe :one
INSERT INTO recipes (name) VALUES (?) RETURNING *;

-- name: UpdateRecipeName :exec
UPDATE recipes SET name = ? WHERE id = ?;

-- name: DeleteRecipe :exec
DELETE FROM recipes WHERE id = ?;

-- name: GetRecipe :one
SELECT * FROM recipes WHERE id = ?;

-- name: ListRecipes :many
SELECT
  r.id,
  r.name,
  r.created_at,
  CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories,
  CAST(COALESCE(SUM(f.protein_per_unit  * ri.quantity), 0) AS REAL) AS total_protein
FROM recipes r
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN foods f               ON f.id          = ri.food_id
WHERE r.name LIKE '%' || sqlc.arg(search) || '%'
GROUP BY r.id
ORDER BY r.name;

-- name: GetRecipeIngredients :many
SELECT
  ri.id,
  ri.recipe_id,
  ri.food_id,
  ri.quantity,
  f.name              AS food_name,
  f.unit              AS food_unit,
  f.calories_per_unit AS calories_per_unit,
  f.protein_per_unit  AS protein_per_unit
FROM recipe_ingredients ri
JOIN foods f ON f.id = ri.food_id
WHERE ri.recipe_id = ?
ORDER BY ri.id;

-- name: AddRecipeIngredient :one
INSERT INTO recipe_ingredients (recipe_id, food_id, quantity)
VALUES (?, ?, ?)
RETURNING *;

-- name: ClearRecipeIngredients :exec
DELETE FROM recipe_ingredients WHERE recipe_id = ?;
