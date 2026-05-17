-- name: UpsertMetrics :one
INSERT INTO daily_metrics (user_id, date, weight, steps, target_calories)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id, date) DO UPDATE SET
  weight          = excluded.weight,
  steps           = excluded.steps,
  target_calories = excluded.target_calories
RETURNING *;

-- name: GetMetricsRange :many
SELECT * FROM daily_metrics
WHERE user_id = sqlc.arg(user_id)
  AND date >= sqlc.arg(from_date)
  AND date <= sqlc.arg(to_date)
ORDER BY date;

-- name: GetMetricsForDate :one
SELECT * FROM daily_metrics
WHERE user_id = ? AND date = ?;

-- name: GetTodaySummary :one
SELECT
  CAST(COALESCE((SELECT SUM(le.calories) FROM log_entries le
            WHERE le.user_id = sqlc.arg(user_id) AND le.date = date('now')), 0) AS REAL) AS consumed,
  CAST(COALESCE((SELECT dm.target_calories FROM daily_metrics dm
            WHERE dm.user_id = sqlc.arg(user_id) AND dm.date = date('now')), 0) AS INTEGER) AS target;
