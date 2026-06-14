-- name: ListIngredients :many
SELECT * FROM ingredients
WHERE name LIKE '%' || sqlc.arg(search) || '%'
ORDER BY name;

-- name: GetIngredient :one
SELECT * FROM ingredients WHERE id = ?;

-- name: CreateIngredient :one
INSERT INTO ingredients (name, unit, calories_per_unit, protein_per_unit)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: DeleteIngredient :exec
DELETE FROM ingredients WHERE id = ?;

-- name: UpdateIngredientNutrition :one
UPDATE ingredients
SET calories_per_unit = ?,
    protein_per_unit  = ?
WHERE id = ?
RETURNING *;

-- name: RestampLogEntriesForIngredient :exec
UPDATE log_entries
SET calories_per_unit = ?1,
    calories          = ?1 * quantity,
    protein_per_unit  = ?2,
    protein           = ?2 * quantity
WHERE ingredient_id = ?3;
