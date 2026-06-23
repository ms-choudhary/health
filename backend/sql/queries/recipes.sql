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
LEFT JOIN ingredients f               ON f.id          = ri.ingredient_id
WHERE r.name LIKE '%' || sqlc.arg(search) || '%'
GROUP BY r.id
ORDER BY r.name;

-- name: GetRecipeIngredients :many
SELECT
  ri.id,
  ri.recipe_id,
  ri.ingredient_id,
  ri.quantity,
  f.name              AS ingredient_name,
  f.unit              AS ingredient_unit,
  f.calories_per_unit AS calories_per_unit,
  f.protein_per_unit  AS protein_per_unit
FROM recipe_ingredients ri
JOIN ingredients f ON f.id = ri.ingredient_id
WHERE ri.recipe_id = ?
ORDER BY ri.id;

-- name: GetRecipesByIngredient :many
SELECT DISTINCT r.id, r.name
FROM recipes r
JOIN recipe_ingredients ri ON ri.recipe_id = r.id
WHERE ri.ingredient_id = ?
ORDER BY r.name;

-- name: AddRecipeIngredient :one
INSERT INTO recipe_ingredients (recipe_id, ingredient_id, quantity)
VALUES (?, ?, ?)
RETURNING *;

-- name: ClearRecipeIngredients :exec
DELETE FROM recipe_ingredients WHERE recipe_id = ?;
