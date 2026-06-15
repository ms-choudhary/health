# Plan: Add Exercise Tracking

This document is an implementation plan for adding **exercise tracking** alongside the
existing food/nutrition tracking. It mirrors the architecture already documented in
`research.md`: an **Exercise Library** parallel to the Food Library, exercise **tags**
parallel to food tags, and **sets** (weight × reps) parallel to recipe log entries.

> **Styling note (important):** The reference mockup at
> `workout-tracker-mockup.html` defines the **UX/layout** (tag tiles → exercise cards →
> multi-set builder, sets grouped by date in Logs, per-exercise bar charts). We adopt
> that *interaction model* but **keep the existing app theme** — the shadcn-vue CSS
> variables, `Card`/`Button`/`Dialog`/`Badge`/`Input` primitives, and the current
> blue/amber/green/violet chart palette. Do **not** import the mockup's lime/black
> colors, fonts (`DM Sans`/`JetBrains Mono`), or bespoke CSS classes.

---

## 1. Goals (from the request)

1. **Exercise Library** — a separate library (like Food Library) where you manage
   **Exercises** and exercise-specific **Tags**.
2. **Today page** — two sections, **Food** and **Exercises**, each showing *its own*
   tags. Tapping a tag reveals its items; for exercises you can **add sets** (weight +
   reps, multi-set builder per the mockup). When you open an exercise's "Add Sets"
   drawer, it is **prefilled with that exercise's most recently recorded sets** for the
   user (so a typical session is "tweak last time's numbers and save"). If the exercise
   has never been logged, it starts with a single default set.
3. **User screen** — do **not** add "Exercises"/"Tags" tabs; that management lives in
   the Exercise Library. The user screen keeps its 3 tabs (Today / Progress / Log).
4. **Progress page** — a collapsible (default-folded) **"Exercises"** section with one
   **bar chart per recorded exercise**; exercises with no data in range are omitted.
5. **Logs page** — add a **sub-tab for exercises** (Food | Exercises), showing logged
   sets grouped by date.

---

## 2. Data model & key decisions

We reuse the proven "library → tagged items → snapshotted log rows" pattern.

| Food world (exists)            | Exercise world (new)              |
|--------------------------------|-----------------------------------|
| `ingredients`                  | `exercises`                       |
| `food_tags`                    | `exercise_tags`                   |
| `recipe_food_tags` (M:N)       | `exercise_taggings` (M:N)    |
| `log_entries` (recipe → rows)  | `exercise_sets` (exercise → rows) |
| `recipes` + `recipe_ingredients` | *(no equivalent — sets are logged directly against an exercise)* |

Decisions:

- **Exercises are simpler than recipes.** An exercise has a `name`, optional `notes`
  (the mockup shows notes like "Aim for 40", "3 sets"), and tags. There is no
  composition step (no "recipe of exercises"); you log sets directly against an
  exercise. So there is **no `exercise_ingredients`-style junction**.
- **Sets snapshot the exercise name** (`exercise_name`) like `log_entries` snapshots
  ingredient data, and `exercise_id` is `ON DELETE SET NULL` so deleting an exercise
  from the library preserves logged history.
- **Per-set fields:** `weight` (REAL), `reps` (INTEGER), `unit` (TEXT default `'kg'`,
  effectively constant but kept for future-proofing), `date` (client-supplied
  `YYYY-MM-DD`, for grouping/charts). **No `logged_at`/timestamp column** — sets are
  ordered within a day by insertion order (`id`), and the Logs tab groups by `date`
  without showing a time-of-day. (The mockup shows per-set times, but we deliberately
  drop that — it adds a UTC-vs-local wrinkle for no real value here.)
- **Exercise tags are a separate taxonomy** from food tags (own table), so "Breakfast"
  and "upper" never collide. Case-insensitive uniqueness, like `food_tags`.
- **Progress metric:** per exercise, plot **total volume per day** (Σ weight·reps
  across all of that day's sets) as bars over the selected period. Rising volume =
  progress. Each bar's **hover/tooltip shows that day's set breakdown** as
  `weight`×`reps` pairs, space-separated, in set order — e.g. `22.5x7 22.5x8 22.5x3`
  (three sets). To support this, the progress endpoint returns, per exercise per day,
  both the summed `total_volume` and the ordered list of that day's sets used to build
  the breakdown string.

> **Gotcha — two schema files:** as noted in `research.md §6.4`, `backend/sql/schema.sql`
> (sqlc source) and `backend/db/schema.sql` (the `//go:embed`-ed runtime copy) must be
> edited **identically**. New tables use `CREATE TABLE IF NOT EXISTS`, so no `db.go`
> `ensureColumn` migration is needed — they're created on next boot. Only add
> `ensureColumn`/`dropColumnIfExists` calls if you later alter an existing table.

---

## 3. Backend changes

### 3.1 Schema additions

Append to **both** `backend/sql/schema.sql` **and** `backend/db/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS exercises (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  notes      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS exercise_tags (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL COLLATE NOCASE UNIQUE,
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS exercise_taggings (
  exercise_id     INTEGER NOT NULL REFERENCES exercises(id)     ON DELETE CASCADE,
  exercise_tag_id INTEGER NOT NULL REFERENCES exercise_tags(id) ON DELETE CASCADE,
  PRIMARY KEY (exercise_id, exercise_tag_id)
);

CREATE TABLE IF NOT EXISTS exercise_sets (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id)     ON DELETE CASCADE,
  exercise_id   INTEGER          REFERENCES exercises(id) ON DELETE SET NULL,
  exercise_name TEXT    NOT NULL,
  date          TEXT    NOT NULL,
  weight        REAL    NOT NULL DEFAULT 0,
  reps          INTEGER NOT NULL DEFAULT 0,
  unit          TEXT    NOT NULL DEFAULT 'kg'
);

CREATE INDEX IF NOT EXISTS idx_sets_user_date        ON exercise_sets(user_id, date);
CREATE INDEX IF NOT EXISTS idx_sets_exercise         ON exercise_sets(exercise_id);
CREATE INDEX IF NOT EXISTS idx_taggings_tag          ON exercise_taggings(exercise_tag_id);
```

### 3.2 sqlc queries

Create `backend/sql/queries/exercises.sql`:

```sql
-- name: ListExercises :many
SELECT * FROM exercises
WHERE name LIKE '%' || sqlc.arg(search) || '%'
ORDER BY name;

-- name: GetExercise :one
SELECT * FROM exercises WHERE id = ?;

-- name: CreateExercise :one
INSERT INTO exercises (name, notes) VALUES (?, ?) RETURNING *;

-- name: UpdateExercise :one
UPDATE exercises SET name = ?, notes = ? WHERE id = ? RETURNING *;

-- name: DeleteExercise :exec
DELETE FROM exercises WHERE id = ?;
```

Create `backend/sql/queries/exercise_tags.sql` (mirror of `food_tags.sql`):

```sql
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
```

Create `backend/sql/queries/sets.sql`:

```sql
-- name: AddSet :one
INSERT INTO exercise_sets (user_id, exercise_id, exercise_name, date, weight, reps, unit)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetSetHistory :many
SELECT * FROM exercise_sets
WHERE user_id = ?
ORDER BY date DESC, id DESC;

-- name: GetLastSetsForExercise :many
-- The sets from the most recent day this user logged this exercise (used to
-- prefill the "Add Sets" drawer). Empty when never logged.
SELECT * FROM exercise_sets
WHERE user_id = sqlc.arg(user_id)
  AND exercise_id = sqlc.arg(exercise_id)
  AND date = (
    SELECT MAX(date) FROM exercise_sets
    WHERE user_id = sqlc.arg(user_id) AND exercise_id = sqlc.arg(exercise_id)
  )
ORDER BY id;

-- name: UpdateSet :one
UPDATE exercise_sets SET weight = ?, reps = ?
WHERE id = ? AND user_id = ?
RETURNING *;

-- name: DeleteSet :exec
DELETE FROM exercise_sets WHERE id = ? AND user_id = ?;

-- name: GetProgressSets :many
-- Raw per-set rows in range, ordered so Go can group by exercise → date and build
-- both the per-day total volume and the breakdown string ("22.5x7 22.5x8 …").
SELECT s.exercise_id, s.exercise_name, s.date, s.weight, s.reps
FROM exercise_sets s
WHERE s.user_id = sqlc.arg(user_id)
  AND s.date >= sqlc.arg(from_date)
  AND s.date <= sqlc.arg(to_date)
ORDER BY s.exercise_name COLLATE NOCASE, s.date, s.id;
```

Regenerate with `sqlc generate` (run in `backend/`). This writes
`db/queries/exercises.sql.go`, `exercise_tags.sql.go`, `sets.sql.go` and adds the new
structs to `models.go`.

### 3.3 Handlers

New files under `backend/handlers/`, following the existing conventions exactly
(`parseID`, `readJSON`, `writeJSON`/`writeError`, transactions via `h.DB.BeginTx` +
`h.Q.WithTx(tx)`, `validDate`).

**`exercises.go`** — mirror `recipes.go`'s tag-application + transactional create/update.

```go
type exerciseBody struct {
	Name          string  `json:"name"`
	Notes         string  `json:"notes"`
	ExerciseTagIDs []int64 `json:"exercise_tag_ids"`
}

type exerciseDetailResponse struct {
	queries.Exercise
	Tags []queries.ExerciseTag `json:"tags"`
}

// applyExerciseTags validates each tag exists then INSERT OR IGNORE links it,
// exactly like recipes.go:applyRecipeFoodTags.
func (h *Handler) applyExerciseTags(w http.ResponseWriter, r *http.Request,
	q *queries.Queries, exerciseID int64, tagIDs []int64) bool { /* ...same shape... */ }

func (h *Handler) CreateExercise(w http.ResponseWriter, r *http.Request) {
	var body exerciseBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error()); return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" { writeError(w, http.StatusBadRequest, "name required"); return }

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	ex, err := q.CreateExercise(r.Context(), queries.CreateExerciseParams{
		Name: name, Notes: strings.TrimSpace(body.Notes),
	})
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	if !h.applyExerciseTags(w, r, q, ex.ID, body.ExerciseTagIDs) { return }
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error()); return
	}
	writeJSON(w, http.StatusCreated, ex)
}
// ListExercises / GetExercise (with tags) / UpdateExercise (clear+reapply tags, tx) /
// DeleteExercise — all mirror recipes.go.
```

**`exercise_tags.go`** — a near-verbatim copy of `food_tags.go` with the
`ExerciseTag`/`exercise_count`/`ListAllExerciseTagLinks` query names. Includes
`ListExerciseTags`, `CreateExerciseTag` (409 on duplicate via `GetExerciseTagByName`),
`DeleteExerciseTag`, `GetExercisesByTag`, and an `exerciseTagMap(r)` helper to attach
tags to exercise lists (same shape as `recipeFoodTagMap`).

**`sets.go`** — mirror `log.go` + `LogRecipe`.

```go
type setInput struct {
	Weight float64 `json:"weight"`
	Reps   int64   `json:"reps"`
}
type addSetsBody struct {
	ExerciseID int64      `json:"exercise_id"`
	Date       string     `json:"date"`
	Unit       string     `json:"unit"`
	Sets       []setInput `json:"sets"`
}

func (h *Handler) AddSets(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	var body addSetsBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error()); return
	}
	if body.ExerciseID <= 0 { writeError(w, http.StatusBadRequest, "exercise_id required"); return }
	if !validDate(body.Date) { writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD"); return }
	if len(body.Sets) == 0 { writeError(w, http.StatusBadRequest, "at least one set required"); return }
	for _, s := range body.Sets {
		if s.Weight < 0 || s.Reps < 0 {
			writeError(w, http.StatusBadRequest, "weight/reps must be >= 0"); return
		}
	}
	ex, err := h.Q.GetExercise(r.Context(), body.ExerciseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { writeError(w, http.StatusNotFound, "exercise not found"); return }
		writeError(w, http.StatusInternalServerError, err.Error()); return
	}
	unit := strings.TrimSpace(body.Unit)
	if unit == "" { unit = "kg" }

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	out := make([]queries.ExerciseSet, 0, len(body.Sets))
	for _, s := range body.Sets {
		exID := ex.ID
		row, err := q.AddSet(r.Context(), queries.AddSetParams{
			UserID: userID, ExerciseID: &exID, ExerciseName: ex.Name,
			Date: body.Date, Weight: s.Weight, Reps: s.Reps, Unit: unit,
		})
		if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
		out = append(out, row)
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error()); return
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetSets   -> q.GetSetHistory (returns [] when nil, like GetLog)
// GetLastSets (prefill) -> see below
// UpdateSet -> validate weight/reps >= 0, q.UpdateSet
// DeleteSet -> q.DeleteSet (uses {id} + {sid})
// GetExerciseProgress -> see below

// GetLastSets backs the Add-Sets prefill. Returns the most-recent day's sets for the
// exercise (possibly empty), as plain {weight, reps} the drawer can drop straight in.
func (h *Handler) GetLastSets(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	exerciseID, err := parseID(r, "eid")
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	rows, err := h.Q.GetLastSetsForExercise(r.Context(), queries.GetLastSetsForExerciseParams{
		UserID: userID, ExerciseID: &exerciseID,
	})
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	out := make([]setInput, 0, len(rows))   // [] when never logged
	for _, s := range rows {
		out = append(out, setInput{Weight: s.Weight, Reps: s.Reps})
	}
	writeJSON(w, http.StatusOK, out)
}
```

The progress handler reads the raw per-set rows (already ordered by exercise → date →
id) and folds them into per-exercise series, computing each day's **total volume** and a
**breakdown string** (`weight`×`reps` joined by spaces). Only exercises with sets in the
range appear, so empties never reach the chart:

```go
type progressPoint struct {
	Date        string  `json:"date"`
	TotalVolume float64 `json:"total_volume"`
	Breakdown   string  `json:"breakdown"`     // e.g. "22.5x7 22.5x8 22.5x3"
}
type exerciseProgress struct {
	ExerciseID   *int64          `json:"exercise_id"`
	ExerciseName string          `json:"exercise_name"`
	Points       []progressPoint `json:"points"`
}

// trimNum renders 22.5 -> "22.5" and 20 -> "20" (no trailing ".0").
func trimNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func (h *Handler) GetExerciseProgress(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if !validDate(from) || !validDate(to) {
		writeError(w, http.StatusBadRequest, "from/to must be YYYY-MM-DD"); return
	}
	rows, err := h.Q.GetProgressSets(r.Context(), queries.GetProgressSetsParams{
		UserID: userID, FromDate: from, ToDate: to,
	})
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }

	// rows ordered by exercise_name, date, id — fold preserving order.
	out := make([]exerciseProgress, 0)
	exIdx := map[string]int{}        // exercise_name -> index in out (snapshot survives deletes)
	for _, row := range rows {
		ei, ok := exIdx[row.ExerciseName]
		if !ok {
			exIdx[row.ExerciseName] = len(out)
			out = append(out, exerciseProgress{ExerciseID: row.ExerciseID, ExerciseName: row.ExerciseName})
			ei = exIdx[row.ExerciseName]
		}
		pts := out[ei].Points
		// append to the current day or start a new one (rows for a day are contiguous)
		if n := len(pts); n == 0 || pts[n-1].Date != row.Date {
			out[ei].Points = append(pts, progressPoint{Date: row.Date})
			pts = out[ei].Points
		}
		p := &out[ei].Points[len(out[ei].Points)-1]
		p.TotalVolume += row.Weight * float64(row.Reps)
		piece := trimNum(row.Weight) + "x" + strconv.FormatInt(row.Reps, 10)
		if p.Breakdown == "" { p.Breakdown = piece } else { p.Breakdown += " " + piece }
	}
	writeJSON(w, http.StatusOK, out) // only exercises with data in range appear
}
```

> `sets.go` will need `strconv` in its imports (alongside `database/sql`, `errors`,
> `net/http`, `strings`, `health/db/queries`).

### 3.4 Routes (`main.go`)

Add after the existing food-tag / metric routes:

```go
// Exercise library
mux.HandleFunc("GET /api/exercises", h.ListExercises)
mux.HandleFunc("POST /api/exercises", h.CreateExercise)
mux.HandleFunc("GET /api/exercises/{id}", h.GetExercise)
mux.HandleFunc("PUT /api/exercises/{id}", h.UpdateExercise)
mux.HandleFunc("DELETE /api/exercises/{id}", h.DeleteExercise)

// Exercise tags
mux.HandleFunc("GET /api/exercise-tags", h.ListExerciseTags)
mux.HandleFunc("POST /api/exercise-tags", h.CreateExerciseTag)
mux.HandleFunc("DELETE /api/exercise-tags/{id}", h.DeleteExerciseTag)
mux.HandleFunc("GET /api/exercise-tags/{id}/exercises", h.GetExercisesByTag)

// Sets (per-user log)
mux.HandleFunc("GET /api/users/{id}/sets", h.GetSets)
mux.HandleFunc("POST /api/users/{id}/sets", h.AddSets)
mux.HandleFunc("PUT /api/users/{id}/sets/{sid}", h.UpdateSet)
mux.HandleFunc("DELETE /api/users/{id}/sets/{sid}", h.DeleteSet)
mux.HandleFunc("GET /api/users/{id}/exercises/{eid}/last-sets", h.GetLastSets)
mux.HandleFunc("GET /api/users/{id}/exercise-progress", h.GetExerciseProgress)
```

### 3.5 Seed (optional)

Extend `cmd/seed/main.go` to insert a handful of exercises (e.g. "hack squat",
"db lateral raise", "ez bar curl"), exercise tags ("full body", "upper", "lower"), and
~14 days of sets for each user using the existing seeded RNG. Not required for the
feature, but useful for exercising the charts.

---

## 4. Frontend changes

### 4.1 Types (`src/lib/types.ts`)

```ts
export interface Exercise {
  id: number
  name: string
  notes: string
  created_at: string
}
export interface ExerciseTag {
  id: number
  name: string
  created_at: string
}
export interface ExerciseTagWithCount extends ExerciseTag {
  exercise_count: number
}
export interface ExerciseWithTags extends Exercise {
  tags: ExerciseTag[]
}
export interface ExerciseSet {
  id: number
  user_id: number
  exercise_id: number | null
  exercise_name: string
  date: string
  weight: number
  reps: number
  unit: string
}
export interface SetInput { weight: number; reps: number }
export interface AddSetsPayload {
  exercise_id: number
  date: string
  unit?: string
  sets: SetInput[]
}
export interface ExercisePayload {
  name: string
  notes: string
  exercise_tag_ids: number[]
}
export interface ProgressPoint {
  date: string
  total_volume: number      // Σ weight·reps for the day — the bar value
  breakdown: string         // e.g. "22.5x7 22.5x8 22.5x3" — shown on hover
}
export interface ExerciseProgress {
  exercise_id: number | null
  exercise_name: string
  points: ProgressPoint[]
}
```

### 4.2 API client (`src/lib/api.ts`)

```ts
// Exercises
listExercises: (search = '') =>
  request<ExerciseWithTags[]>(`${BASE}/exercises?q=${encodeURIComponent(search)}`),
getExercise: (id: number) => request<ExerciseWithTags>(`${BASE}/exercises/${id}`),
createExercise: (p: ExercisePayload) =>
  request<Exercise>(`${BASE}/exercises`, { method: 'POST', body: JSON.stringify(p) }),
updateExercise: (id: number, p: ExercisePayload) =>
  request<Exercise>(`${BASE}/exercises/${id}`, { method: 'PUT', body: JSON.stringify(p) }),
deleteExercise: (id: number) =>
  request<void>(`${BASE}/exercises/${id}`, { method: 'DELETE' }),

// Exercise tags
listExerciseTags: () => request<ExerciseTagWithCount[]>(`${BASE}/exercise-tags`),
createExerciseTag: (name: string) =>
  request<ExerciseTag>(`${BASE}/exercise-tags`, { method: 'POST', body: JSON.stringify({ name }) }),
deleteExerciseTag: (id: number) =>
  request<void>(`${BASE}/exercise-tags/${id}`, { method: 'DELETE' }),
exercisesByTag: (tagId: number) =>
  request<ExerciseWithTags[]>(`${BASE}/exercise-tags/${tagId}/exercises`),

// Sets
getSets: (userId: number) => request<ExerciseSet[]>(`${BASE}/users/${userId}/sets`),
lastSets: (userId: number, exerciseId: number) =>
  request<SetInput[]>(`${BASE}/users/${userId}/exercises/${exerciseId}/last-sets`),
addSets: (userId: number, p: AddSetsPayload) =>
  request<ExerciseSet[]>(`${BASE}/users/${userId}/sets`, { method: 'POST', body: JSON.stringify(p) }),
updateSet: (userId: number, setId: number, p: SetInput) =>
  request<ExerciseSet>(`${BASE}/users/${userId}/sets/${setId}`, { method: 'PUT', body: JSON.stringify(p) }),
deleteSet: (userId: number, setId: number) =>
  request<void>(`${BASE}/users/${userId}/sets/${setId}`, { method: 'DELETE' }),
exerciseProgress: (userId: number, from: string, to: string) =>
  request<ExerciseProgress[]>(`${BASE}/users/${userId}/exercise-progress?from=${from}&to=${to}`),
```

### 4.3 Router + Home entry point

`src/router/index.ts` — add the exercise library route:

```ts
{ path: '/exercise-library', name: 'exercise-library',
  component: () => import('@/views/ExerciseLibrary.vue') },
```

`src/views/Home.vue` — add an "Exercise Library" button beside the existing "Food
Library" button in the header (reuse the `Button variant="outline" size="sm"` style;
use a `Dumbbell` icon from `lucide-vue-next`):

```vue
<div class="flex gap-2">
  <Button variant="outline" size="sm" @click="router.push('/library')">
    <BookOpen class="h-4 w-4" /> Food
  </Button>
  <Button variant="outline" size="sm" @click="router.push('/exercise-library')">
    <Dumbbell class="h-4 w-4" /> Exercises
  </Button>
</div>
```

### 4.4 New: `src/views/ExerciseLibrary.vue`

A trimmed copy of `IngredientLibrary.vue` with **two** sub-tabs: `Exercises` | `Tags`
(no "Recipes" tab — exercises aren't composed). Reuses the same debounced-search,
`confirm()`-to-delete, and editor-dialog patterns.

- **Exercises tab:** searchable `Card` list; each row shows name, notes (muted), and tag
  `Badge`s; pencil → `ExerciseEditor`, trash → `deleteExercise`. "Add exercise" button.
- **Tags tab:** mirrors `IngredientLibrary`'s tags tab — list `exercise_count`, drill
  into a tag to list its exercises (`exercisesByTag`), "Add tag" → `ExerciseTagEditor`.

State/loaders mirror `IngredientLibrary.vue` (`loadExercises`, `loadExerciseTags`,
`openFoodTag`→`openTag`, etc.).

### 4.5 New: `ExerciseEditor.vue` and `ExerciseTagEditor.vue`

- **`ExerciseTagEditor.vue`** — a near-verbatim copy of `FoodTagEditor.vue` calling
  `api.createExerciseTag`. Props `open`; emits `update:open`, `saved`.
- **`ExerciseEditor.vue`** — model on `RecipeEditor.vue` minus the ingredient list:
  fields are **name**, **notes** (an `Input` or short textarea), and **toggleable tag
  pills** (`availableExerciseTags` from `listExerciseTags`, `selectedTagIds` toggled
  like `RecipeEditor.toggleFoodTag`). Props `open`, `exerciseId: number | null`; emits
  `update:open`, `saved`. Calls `getExercise` (edit), `createExercise`/`updateExercise`.

### 4.6 `TodayTab.vue` — Food + Exercises sections

Refactor `TodayTab` to render **two labeled sections**. Keep the existing food flow
intact; add a parallel exercise flow.

```vue
<script setup lang="ts">
// ...existing imports + AddSetsDrawer, exercise types/api
const foodTags = ref<FoodTagWithCount[]>([])
const exerciseTags = ref<ExerciseTagWithCount[]>([])

// drill-down state, now scoped by kind
const selected = ref<{ kind: 'food' | 'exercise'; tag: { id: number; name: string } } | null>(null)
const recipes = ref<RecipeListItem[]>([])
const exercises = ref<ExerciseWithTags[]>([])

const showRecipeDrawer = ref(false)
const pickedRecipe = ref<RecipeListItem | null>(null)
const showSetsDrawer = ref(false)
const pickedExercise = ref<ExerciseWithTags | null>(null)

async function openFoodTag(t: FoodTagWithCount) {
  selected.value = { kind: 'food', tag: t }
  recipes.value = await api.recipesByFoodTag(t.id)
}
async function openExerciseTag(t: ExerciseTagWithCount) {
  selected.value = { kind: 'exercise', tag: t }
  exercises.value = await api.exercisesByTag(t.id)
}
onMounted(async () => {
  ;[foodTags.value, exerciseTags.value] = await Promise.all([
    api.listFoodTags(), api.listExerciseTags(),
  ])
})
</script>

<template>
  <!-- date header unchanged -->

  <!-- when no tag selected: two stacked sections -->
  <template v-if="!selected">
    <section>
      <h2 class="text-sm font-semibold text-muted-foreground mb-2">Food</h2>
      <div class="grid grid-cols-2 gap-2">
        <button v-for="t in foodTags" :key="t.id" @click="openFoodTag(t)"> … tag tile … </button>
      </div>
    </section>
    <section class="mt-6">
      <h2 class="text-sm font-semibold text-muted-foreground mb-2">Exercises</h2>
      <div class="grid grid-cols-2 gap-2">
        <button v-for="t in exerciseTags" :key="t.id" @click="openExerciseTag(t)">
          <!-- tile shows t.name + `${t.exercise_count} exercises` -->
        </button>
      </div>
    </section>
  </template>

  <!-- food drill-down: recipe cards + "Add to log" -> AddRecipeDrawer (existing) -->
  <!-- exercise drill-down: exercise cards + "Add Sets" -> AddSetsDrawer (new) -->
  <template v-else-if="selected.kind === 'exercise'">
    <Button variant="ghost" size="sm" @click="selected = null">← Back</Button>
    <Card v-for="ex in exercises" :key="ex.id">
      <div class="p-3 flex items-center justify-between gap-3">
        <div>
          <div class="font-medium">{{ ex.name }}</div>
          <div v-if="ex.notes" class="text-xs text-muted-foreground italic">{{ ex.notes }}</div>
          <div class="mt-1 flex flex-wrap gap-1">
            <Badge v-for="t in ex.tags" :key="t.id" variant="outline">{{ t.name }}</Badge>
          </div>
        </div>
        <Button size="sm" @click="pickedExercise = ex; showSetsDrawer = true">Add Sets</Button>
      </div>
    </Card>
  </template>

  <AddSetsDrawer
    v-if="showSetsDrawer && pickedExercise"
    :user-id="userId" :date="today" :exercise="pickedExercise"
    @close="showSetsDrawer = false"
    @added="showSetsDrawer = false /* + optional toast */" />
</template>
```

Tag tiles should reuse the existing food-tile markup/classes (Card-like, `Badge` for
count) so the two sections look identical — **not** the mockup's `.tag-tile` CSS.

### 4.7 New: `AddSetsDrawer.vue`

A bottom-sheet modeled on `AddRecipeDrawer.vue` (teleport to `body`, dark overlay,
`rounded-t-2xl`, Escape-to-close, `visualViewport` keyboard handling, body-scroll lock).
Inside, it reimplements the mockup's **multi-set builder** with the app's primitives.

```vue
<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { api } from '@/lib/api'
import type { ExerciseWithTags } from '@/lib/types'
import Button from '@/components/ui/Button.vue'
import { Plus, Minus, Trash2 } from 'lucide-vue-next'

const props = defineProps<{ userId: number; date: string; exercise: ExerciseWithTags }>()
const emit = defineEmits<{ close: []; added: [] }>()

interface Draft { id: number; weight: number; reps: number }
let counter = 0
const sets = reactive<Draft[]>([])
const saving = ref(false)
const loading = ref(true)
const errMsg = ref('')

// Prefill with the exercise's most recently recorded sets (per the Today-page note).
// Falls back to a single default set when the exercise has never been logged.
onMounted(async () => {
  try {
    const last = await api.lastSets(props.userId, props.exercise.id)
    if (last.length) {
      for (const s of last) sets.push({ id: counter++, weight: s.weight, reps: s.reps })
    }
  } catch {
    /* ignore — fall through to default */
  } finally {
    if (sets.length === 0) sets.push({ id: counter++, weight: 20, reps: 8 })
    loading.value = false
  }
})

function addSet() {
  const last = sets[sets.length - 1]
  sets.push({ id: counter++, weight: last?.weight ?? 20, reps: last?.reps ?? 8 })
}
function removeSet(id: number) {
  const i = sets.findIndex((s) => s.id === id)
  if (i >= 0) sets.splice(i, 1)
  if (sets.length === 0) addSet()
}
function step(s: Draft, field: 'weight' | 'reps', delta: number) {
  if (field === 'weight') s.weight = Math.max(0, +(s.weight + delta).toFixed(1))
  else s.reps = Math.max(0, s.reps + delta)
}

async function create() {
  saving.value = true; errMsg.value = ''
  try {
    await api.addSets(props.userId, {
      exercise_id: props.exercise.id,
      date: props.date,
      sets: sets.map((s) => ({ weight: s.weight, reps: s.reps })),
    })
    emit('added')
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to add sets'
  } finally { saving.value = false }
}
</script>

<template>
  <!-- Teleport + overlay + bottom sheet (copy AddRecipeDrawer's shell) -->
  <div class="text-sm text-muted-foreground mb-1">Add sets</div>
  <div class="font-semibold mb-3">{{ exercise.name }}</div>

  <div class="flex flex-col gap-2">
    <div v-for="(s, idx) in sets" :key="s.id" class="rounded-lg border border-border p-3">
      <div class="flex items-center justify-between mb-2">
        <span class="text-xs font-medium text-muted-foreground uppercase">Set {{ idx + 1 }}</span>
        <Button v-if="sets.length > 1" variant="ghost" size="icon" @click="removeSet(s.id)">
          <Trash2 class="h-4 w-4" />
        </Button>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <!-- Weight stepper (±2.5) -->
        <div>
          <div class="text-xs text-muted-foreground mb-1">Weight (kg)</div>
          <div class="flex items-stretch gap-1">
            <Button variant="outline" size="icon" @click="step(s, 'weight', -2.5)"><Minus class="h-4 w-4"/></Button>
            <div class="flex-1 grid place-items-center rounded-md border border-input bg-card font-semibold">{{ s.weight }}</div>
            <Button variant="outline" size="icon" @click="step(s, 'weight', 2.5)"><Plus class="h-4 w-4"/></Button>
          </div>
        </div>
        <!-- Reps stepper (±1) -->
        <div>
          <div class="text-xs text-muted-foreground mb-1">Reps</div>
          <div class="flex items-stretch gap-1">
            <Button variant="outline" size="icon" @click="step(s, 'reps', -1)"><Minus class="h-4 w-4"/></Button>
            <div class="flex-1 grid place-items-center rounded-md border border-input bg-card font-semibold">{{ s.reps }}</div>
            <Button variant="outline" size="icon" @click="step(s, 'reps', 1)"><Plus class="h-4 w-4"/></Button>
          </div>
        </div>
      </div>
    </div>
  </div>

  <Button variant="outline" class="w-full mt-2 border-dashed" @click="addSet">
    <Plus class="h-4 w-4" /> Add another set
  </Button>
  <p v-if="errMsg" class="text-sm text-destructive mt-2">{{ errMsg }}</p>
  <Button class="w-full mt-3" :disabled="saving" @click="create">
    {{ saving ? 'Saving…' : 'Create' }}
  </Button>
</template>
```

> Stepper increments come from the mockup: **weight ±2.5**, **reps ±1**, new set
> defaults to the previous set's values. The mockup also supports inline editing of a
> saved set — wire `Edit` to `api.updateSet` later if desired (out of scope for v1; the
> Logs tab supports delete).

### 4.8 `ProgressTab.vue` + new `ExerciseProgressCharts.vue`

In `ProgressTab.vue`, after the existing nutrition `ProgressCharts`, add a **collapsible
"Exercises" section** (default folded). Reuse the existing `period` (`w`/`m`/`yr`) and
its `dateRangeForPeriod()` to fetch progress.

```vue
<script setup lang="ts">
// ...existing
const showExercises = ref(false)            // folded by default
const exerciseProgress = ref<ExerciseProgress[]>([])

async function loadExerciseProgress() {
  const { from, to } = dateRangeForPeriod(period.value)  // reuse existing helper
  exerciseProgress.value = await api.exerciseProgress(props.userId, from, to)
}
watch([period], loadExerciseProgress)
onMounted(loadExerciseProgress)
</script>

<template>
  <!-- existing goals / today / nutrition charts unchanged -->

  <Card class="mt-4">
    <button class="w-full p-3 flex items-center justify-between" @click="showExercises = !showExercises">
      <span class="font-semibold">Exercises</span>
      <ChevronDown class="h-4 w-4 transition-transform" :class="showExercises && 'rotate-180'" />
    </button>
    <div v-if="showExercises" class="p-3 pt-0">
      <ExerciseProgressCharts :data="exerciseProgress" />
    </div>
  </Card>
</template>
```

`ExerciseProgressCharts.vue` — model on `ProgressCharts.vue`: register Chart.js, read
theme colors via `getCssVar('--chart-violet')`, and render **one `<Bar>` per exercise**.
Each chart plots **total volume per day**; the tooltip's footer shows that day's
**set breakdown** (`p.breakdown`, e.g. `22.5x7 22.5x8 22.5x3`). Because the API already
omits exercises with no data, just map over `props.data`; show "No exercise data in this
period" when the array is empty.

```vue
<script setup lang="ts">
import { computed } from 'vue'
import { Bar } from 'vue-chartjs'
import { Chart as ChartJS, CategoryScale, LinearScale, BarElement, Tooltip } from 'chart.js'
import type { ExerciseProgress } from '@/lib/types'
ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip)

const props = defineProps<{ data: ExerciseProgress[] }>()
function getCssVar(name: string) {
  if (typeof document === 'undefined') return 'hsl(221 83% 53%)'
  return `hsl(${getComputedStyle(document.documentElement).getPropertyValue(name).trim()})`
}
const charts = computed(() =>
  props.data.map((ex) => ({
    name: ex.exercise_name,
    chartData: {
      labels: ex.points.map((p) => p.date.slice(5)),        // MM-DD
      datasets: [{
        label: 'Volume (kg)',
        data: ex.points.map((p) => p.total_volume),         // bar height = Σ weight·reps
        breakdown: ex.points.map((p) => p.breakdown),       // parallel array for the tooltip
        backgroundColor: getCssVar('--chart-violet'),
        borderRadius: 4,
      }],
    },
  })),
)
const options = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        // title = date; body = volume; footer = the "22.5x7 22.5x8 …" breakdown
        label: (ctx: any) => `Volume: ${Math.round(ctx.parsed.y)}`,
        footer: (items: any[]) => {
          const it = items[0]
          return it?.dataset?.breakdown?.[it.dataIndex] ?? ''
        },
      },
    },
  },
  scales: { y: { beginAtZero: true } },
}
</script>

<template>
  <div v-if="data.length === 0" class="text-center py-6 text-sm text-muted-foreground">
    No exercise data in this period.
  </div>
  <div v-else class="flex flex-col gap-4">
    <Card v-for="c in charts" :key="c.name">
      <div class="p-3">
        <div class="text-sm font-medium mb-2">{{ c.name }}</div>
        <div class="h-40"><Bar :data="c.chartData" :options="options" /></div>
      </div>
    </Card>
  </div>
</template>
```

> The `breakdown` string array is stashed on the dataset so the tooltip `footer`
> callback can look it up by `dataIndex` — Chart.js ignores unknown dataset keys, so this
> is a clean way to carry per-bar metadata without a second hidden dataset.

### 4.9 `LogTab.vue` — Food | Exercises sub-tabs

Wrap the current content in a **Food** sub-tab and add an **Exercises** sub-tab. Use the
same in-card segmented control already used for tabs elsewhere.

```vue
<script setup lang="ts">
// ...existing recipe-log state
type Sub = 'food' | 'exercises'
const sub = ref<Sub>('food')
const sets = ref<ExerciseSet[]>([])
const loadingSets = ref(false)

interface SetDay { date: string; displayDate: string; rows: ExerciseSet[] }
const setDays = computed<SetDay[]>(() => {
  const byDate = new Map<string, ExerciseSet[]>()
  for (const s of sets.value) (byDate.get(s.date) ?? byDate.set(s.date, []).get(s.date)!).push(s)
  return [...byDate.entries()].map(([date, rows]) => ({
    date, displayDate: format(parseISO(date), 'MMM d, yyyy'), rows,
  }))     // sets already arrive date DESC, id DESC from the API
})

async function loadSets() {
  loadingSets.value = true
  try { sets.value = await api.getSets(props.userId) } finally { loadingSets.value = false }
}
async function removeSet(id: number) {
  if (!confirm('Delete this set?')) return
  await api.deleteSet(props.userId, id); await loadSets()
}
watch(sub, (s) => { if (s === 'exercises' && sets.value.length === 0) loadSets() })
</script>

<template>
  <nav class="flex gap-1 mb-4 p-1 rounded-xl bg-card border border-border">
    <button class="flex-1 h-8 rounded-lg text-sm"
      :class="sub === 'food' ? 'bg-primary text-primary-foreground font-bold' : 'text-muted-foreground'"
      @click="sub = 'food'">Food</button>
    <button class="flex-1 h-8 rounded-lg text-sm"
      :class="sub === 'exercises' ? 'bg-primary text-primary-foreground font-bold' : 'text-muted-foreground'"
      @click="sub = 'exercises'">Exercises</button>
  </nav>

  <template v-if="sub === 'food'"> <!-- existing grouped recipe log markup --> </template>

  <template v-else>
    <div v-if="loadingSets"> <!-- skeletons --> </div>
    <div v-else-if="sets.length === 0" class="text-center py-10 text-muted-foreground text-sm">
      No sets logged yet.
    </div>
    <template v-else v-for="day in setDays" :key="day.date">
      <div class="text-xs uppercase text-muted-foreground border-b border-border pt-5 pb-2">
        {{ day.displayDate }}
      </div>
      <div v-for="s in day.rows" :key="s.id" class="flex items-center gap-3 py-3 border-b border-border">
        <span class="flex-1 truncate text-sm font-medium">{{ s.exercise_name }}</span>
        <span class="font-mono text-sm">
          <span class="font-semibold">{{ s.weight }}</span><span class="text-xs text-muted-foreground">{{ s.unit }}</span>
          <span class="text-muted-foreground"> × </span><span class="font-semibold">{{ s.reps }}</span>
        </span>
        <Button variant="ghost" size="icon" @click="removeSet(s.id)"><Trash2 class="h-4 w-4" /></Button>
      </div>
    </template>
  </template>
</template>
```

> Sets are grouped by `date` only — **no time-of-day** is shown (we dropped `logged_at`;
> see §2). Within a day, rows render in insertion order (`id`), most-recent day first.

---

## 5. Implementation order (checklist)

**Backend**
1. [ ] Add the four tables + indexes to **both** `sql/schema.sql` and `db/schema.sql`.
2. [ ] Add `sql/queries/exercises.sql`, `exercise_tags.sql`, `sets.sql`.
3. [ ] Run `sqlc generate`; confirm new structs/params in `db/queries/`.
4. [ ] Add `handlers/exercises.go`, `exercise_tags.go`, `sets.go`.
5. [ ] Register routes in `main.go`.
6. [ ] `go vet ./... && go build .` — both must pass (baseline is currently clean).
7. [ ] (optional) extend `cmd/seed/main.go`.

**Frontend**
8. [ ] Extend `lib/types.ts` and `lib/api.ts`.
9. [ ] Add `/exercise-library` route + Home button.
10. [ ] `views/ExerciseLibrary.vue`, `components/ExerciseEditor.vue`, `components/ExerciseTagEditor.vue`.
11. [ ] `components/AddSetsDrawer.vue`.
12. [ ] Refactor `components/TodayTab.vue` into Food + Exercises sections.
13. [ ] `components/ExerciseProgressCharts.vue` + collapsible section in `ProgressTab.vue`.
14. [ ] Add Food | Exercises sub-tabs to `components/LogTab.vue`.
15. [ ] `npm run build` (runs `vue-tsc -b` type-check + `vite build`) — must pass.

**Verify end-to-end**
16. [ ] Create exercise tags + exercises in the Exercise Library.
17. [ ] On Today, open an exercise tag → "Add Sets". First time: starts with one default
        set. After saving, reopen → the drawer is **prefilled with the last session's sets**.
18. [ ] Logs → Exercises sub-tab shows the sets grouped by date (no time-of-day).
19. [ ] Progress → expand "Exercises" → one **total-volume** bar chart per logged
        exercise; hovering a bar shows the set breakdown (`22.5x7 22.5x8 …`); exercises
        with no data in range are absent.
20. [ ] Delete an exercise from the library → past sets remain (name preserved,
        `exercise_id` becomes null).

---

## 6. Notes, edge cases & gotchas

- **Keep both schema files in sync** (`research.md §6.4`). The runtime applies the
  embedded copy; sqlc reads the other. Diverging them causes codegen/runtime drift.
- **History preservation:** `exercise_sets.exercise_id` is `ON DELETE SET NULL` and the
  exercise name is snapshotted, matching `log_entries`. Charts/Logs group on
  `exercise_name` so deleted-exercise history still renders.
- **Separate tag namespaces:** exercise tags live in their own table; the Today page and
  Library never mix them with food tags.
- **No auth** remains app-wide; sets are scoped by `user_id` in every query/route, same
  as log entries.
- **Timezone:** `date` is client-supplied (`validDate` regex, format-only) — identical
  trade-off to the existing food log. There is no `logged_at`/server-time column, so the
  UTC-vs-local time-of-day wrinkle the food log has doesn't apply here.
- **Add-Sets prefill:** the drawer seeds itself from `GetLastSetsForExercise` (the most
  recent `date` the user logged that exercise). If two sessions share that date, all of
  them prefill; never-logged exercises start with one default set.
- **Progress = total volume, with breakdown tooltip:** the bar height is Σ weight·reps
  per day; the breakdown string (`22.5x7 22.5x8 22.5x3`) is built server-side in
  `GetExerciseProgress` and shown in the Chart.js tooltip footer. To switch the bar to a
  different metric (e.g. heaviest set), change the SQL fold + the chart's `data` mapping.
- **Theme only:** reuse existing primitives and CSS variables. The mockup's neon/dark
  palette, custom fonts, and hand-rolled CSS classes are **not** ported.
```
