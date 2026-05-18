-- name: ListFoods :many
SELECT * FROM foods
WHERE name LIKE '%' || sqlc.arg(search) || '%'
ORDER BY name;

-- name: GetFood :one
SELECT * FROM foods WHERE id = ?;

-- name: CreateFood :one
INSERT INTO foods (name, unit, calories_per_unit)
VALUES (?, ?, ?)
RETURNING *;

-- name: DeleteFood :exec
DELETE FROM foods WHERE id = ?;

-- name: UpdateFoodCalories :one
UPDATE foods
SET calories_per_unit = ?
WHERE id = ?
RETURNING *;

-- name: RestampLogEntriesForFood :exec
UPDATE log_entries
SET calories_per_unit = ?1,
    calories          = ?1 * quantity
WHERE food_id = ?2;
