-- name: CreateFoodTag :one
INSERT INTO food_tags (name) VALUES (?) RETURNING *;

-- name: GetFoodTag :one
SELECT * FROM food_tags WHERE id = ?;

-- name: GetFoodTagByName :one
SELECT * FROM food_tags WHERE name = ? COLLATE NOCASE;

-- name: DeleteFoodTag :exec
DELETE FROM food_tags WHERE id = ?;

-- name: ListFoodTags :many
SELECT
  t.id,
  t.name,
  t.created_at,
  CAST(COUNT(rt.recipe_id) AS INTEGER) AS recipe_count
FROM food_tags t
LEFT JOIN recipe_food_tags rt ON rt.food_tag_id = t.id
GROUP BY t.id
ORDER BY t.name COLLATE NOCASE;

-- name: GetRecipeFoodTags :many
SELECT t.id, t.name, t.created_at
FROM recipe_food_tags rt
JOIN food_tags t ON t.id = rt.food_tag_id
WHERE rt.recipe_id = ?
ORDER BY t.name COLLATE NOCASE;

-- name: ListAllRecipeFoodTags :many
SELECT rt.recipe_id, t.id, t.name, t.created_at
FROM recipe_food_tags rt
JOIN food_tags t ON t.id = rt.food_tag_id
ORDER BY t.name COLLATE NOCASE;

-- name: AddRecipeFoodTag :exec
INSERT OR IGNORE INTO recipe_food_tags (recipe_id, food_tag_id) VALUES (?, ?);

-- name: ClearRecipeFoodTags :exec
DELETE FROM recipe_food_tags WHERE recipe_id = ?;

-- name: ListRecipesByFoodTag :many
SELECT
  r.id,
  r.name,
  r.created_at,
  CAST(COALESCE(SUM(i.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories,
  CAST(COALESCE(SUM(i.protein_per_unit  * ri.quantity), 0) AS REAL) AS total_protein
FROM recipes r
JOIN recipe_food_tags rt        ON rt.recipe_id = r.id
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN ingredients i         ON i.id          = ri.ingredient_id
WHERE rt.food_tag_id = ?
GROUP BY r.id
ORDER BY r.name;
