# Add Protein Tracking

## Context

Today the app tracks calories end-to-end: per-food `calories_per_unit`, per-log `calories`, per-recipe live totals, per-user `target_calories`, and a calories line chart on the progress view. The user wants the same first-class treatment for **protein (grams)**:

1. A `protein_per_unit` field on each food (same unit as the food's existing unit).
2. The recipe editor surfaces total **protein per serving** alongside total calories per serving.
3. New users accept a `target_protein` (g), editable from the user page like `target_calories`.
4. A protein-vs-target line chart on the progress view, mirroring the existing calories chart.
5. The daily log table shows protein per row (like Cal).

### Decisions (locked from clarifying questions)

- **Migration**: new columns default to `0` for existing foods, users, and log entries. No backfill — `ALTER TABLE … ADD COLUMN … NOT NULL DEFAULT 0` is the entire migration story.
- **Edit restamp**: editing a food's protein restamps `log_entries.protein_per_unit` and recomputes `log_entries.protein = protein_per_unit × quantity` for that food's rows. Same transaction as the calories edit — they ride together.
- **AI hint**: extend the existing Gemini prompt to ask for both kcal and protein on one line. No response-shape change — the user still reads the hint and types the numbers in manually.
- **Visualization**: a new protein line chart inside `ProgressCharts.vue`, in the same W/M/Yr period view as Calories. It is a **separate card** titled `Protein`, sitting alongside (not merged with) the Calories card — shape, axis behaviour, and options identical to the Calories chart so the two read the same. No protein donut on Home for now.

### What is intentionally **out of scope**

- Editing a food's name or unit (still immutable post-create — same reason as today, see `plans/editable-food.md:7–15`).
- Protein donut on Home or per-day donut on UserPage. Keep the chart story tight; can add later.
- Protein in `GetRecentLoggedFoods` shortlist text. The drawer already shows recent foods by name + last quantity — protein doesn't help discovery.
- Splitting AI into a structured response (kcal/protein parsed out). Free text is fine.

---

## Phase 1 — Schema + migration

### `backend/db/schema.sql` and `backend/sql/schema.sql` (keep both in sync)

Add four columns across three tables:

```sql
CREATE TABLE IF NOT EXISTS users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT    NOT NULL,
  avatar          TEXT    NOT NULL,
  target_calories INTEGER NOT NULL DEFAULT 2000,
  target_protein  INTEGER NOT NULL DEFAULT 0,
  created_at      TEXT    NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS foods (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  name              TEXT NOT NULL,
  unit              TEXT NOT NULL DEFAULT 'g',
  calories_per_unit REAL NOT NULL,
  protein_per_unit  REAL NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS log_entries (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  food_id             INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  date                TEXT NOT NULL,
  food_name           TEXT NOT NULL,
  food_unit           TEXT NOT NULL,
  calories_per_unit   REAL NOT NULL,
  protein_per_unit    REAL NOT NULL DEFAULT 0,
  quantity            REAL NOT NULL,
  calories            REAL NOT NULL,
  protein             REAL NOT NULL DEFAULT 0,
  source_recipe_id    INTEGER,
  source_recipe_name  TEXT
);
```

`recipes` and `recipe_ingredients` are unchanged — recipe protein totals are live-joined from `foods`.

### Migration in `backend/db/db.go`

Add four `ensureColumn` calls before the schema runs. Same idempotent pattern that's already in place for `source_recipe_id`, `source_recipe_name`, and `target_calories` (`db/db.go:30–38`):

```go
if err := ensureColumn(conn, "users", "target_protein", "INTEGER NOT NULL DEFAULT 0"); err != nil {
    return nil, fmt.Errorf("migrate users.target_protein: %w", err)
}
if err := ensureColumn(conn, "foods", "protein_per_unit", "REAL NOT NULL DEFAULT 0"); err != nil {
    return nil, fmt.Errorf("migrate foods.protein_per_unit: %w", err)
}
if err := ensureColumn(conn, "log_entries", "protein_per_unit", "REAL NOT NULL DEFAULT 0"); err != nil {
    return nil, fmt.Errorf("migrate log_entries.protein_per_unit: %w", err)
}
if err := ensureColumn(conn, "log_entries", "protein", "REAL NOT NULL DEFAULT 0"); err != nil {
    return nil, fmt.Errorf("migrate log_entries.protein: %w", err)
}
```

On fresh DBs the tables don't exist yet → `ensureColumn` no-ops → `CREATE TABLE IF NOT EXISTS` brings up the columns from the schema directly. On legacy DBs the columns are backfilled with `0` before subsequent statements run. This is exactly how `target_calories` shipped (`plans/user-config-plan.md:50–53`).

---

## Phase 2 — SQL queries (sqlc)

### `backend/sql/queries/foods.sql`

Extend `CreateFood`, rename `UpdateFoodCalories` → `UpdateFoodNutrition`, and extend `RestampLogEntriesForFood`:

```sql
-- name: CreateFood :one
INSERT INTO foods (name, unit, calories_per_unit, protein_per_unit)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateFoodNutrition :one
UPDATE foods
SET calories_per_unit = ?,
    protein_per_unit  = ?
WHERE id = ?
RETURNING *;

-- name: RestampLogEntriesForFood :exec
UPDATE log_entries
SET calories_per_unit = ?1,
    calories          = ?1 * quantity,
    protein_per_unit  = ?2,
    protein           = ?2 * quantity
WHERE food_id = ?3;
```

`ListFoods`, `GetFood`, `DeleteFood` need no changes (they're `SELECT *` / by-id and the regenerated `Food` struct picks up the new column automatically).

### `backend/sql/queries/log.sql`

Extend `AddLogEntry`, rename `SumCaloriesByDateRange` → `SumNutritionByDateRange`, and update `GetRecentLoggedFoods` (no schema change to the row, but it now reads from a wider table — the existing `food_name/unit/cpu/food_id/last_quantity` projection is fine, we don't need to surface protein here):

```sql
-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, food_id, date, food_name, food_unit,
   calories_per_unit, protein_per_unit,
   quantity, calories, protein,
   source_recipe_id, source_recipe_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

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
```

`GetLogForDate` is `SELECT *` so the regenerated `LogEntry` struct picks up the new columns. `GetRecentLoggedFoods` left alone.

### `backend/sql/queries/recipes.sql`

Extend `ListRecipes` to also compute `total_protein`. `GetRecipeIngredients` projects fields from `foods` — add `protein_per_unit`:

```sql
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
```

`CreateRecipe`, `AddRecipeIngredient`, `ClearRecipeIngredients`, `UpdateRecipeName`, `GetRecipe`, `DeleteRecipe` unchanged.

### `backend/sql/queries/metrics.sql`

Extend `GetTodaySummary` to also return today's protein total. Keep the same row shape pattern:

```sql
-- name: GetTodaySummary :one
SELECT
  CAST(COALESCE((SELECT SUM(le.calories) FROM log_entries le
            WHERE le.user_id = u.id AND le.date = date('now')), 0) AS REAL) AS consumed,
  CAST(COALESCE((SELECT SUM(le.protein) FROM log_entries le
            WHERE le.user_id = u.id AND le.date = date('now')), 0) AS REAL) AS protein_consumed,
  u.target_calories AS target,
  u.target_protein  AS target_protein
FROM users u
WHERE u.id = ?;
```

`UpsertMetrics`, `GetMetricsRange`, `GetMetricsForDate` unchanged.

### `backend/sql/queries/users.sql`

```sql
-- name: CreateUser :one
INSERT INTO users (name, avatar, target_calories, target_protein)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name            = ?,
    target_calories = ?,
    target_protein  = ?
WHERE id = ?
RETURNING *;
```

`ListUsers`, `GetUser`, `DeleteUser` unchanged.

### Regenerate

```bash
cd backend && sqlc generate
```

---

## Phase 3 — Backend handlers

### `backend/handlers/foods.go`

`CreateFood` accepts the new field; `UpdateFood` accepts both nutritional fields and restamps both in one transaction.

```go
type createFoodBody struct {
    Name            string  `json:"name"`
    Unit            string  `json:"unit"`
    CaloriesPerUnit float64 `json:"calories_per_unit"`
    ProteinPerUnit  float64 `json:"protein_per_unit"`
}

func (h *Handler) CreateFood(w http.ResponseWriter, r *http.Request) {
    var body createFoodBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    name := strings.TrimSpace(body.Name)
    if name == "" {
        writeError(w, http.StatusBadRequest, "name required"); return
    }
    unit := strings.TrimSpace(body.Unit)
    if unit == "" { unit = "g" }
    if body.CaloriesPerUnit < 0 {
        writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0"); return
    }
    if body.ProteinPerUnit < 0 {
        writeError(w, http.StatusBadRequest, "protein_per_unit must be >= 0"); return
    }
    food, err := h.Q.CreateFood(r.Context(), queries.CreateFoodParams{
        Name:            name,
        Unit:            unit,
        CaloriesPerUnit: body.CaloriesPerUnit,
        ProteinPerUnit:  body.ProteinPerUnit,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    writeJSON(w, http.StatusCreated, food)
}

type updateFoodBody struct {
    CaloriesPerUnit float64 `json:"calories_per_unit"`
    ProteinPerUnit  float64 `json:"protein_per_unit"`
}

func (h *Handler) UpdateFood(w http.ResponseWriter, r *http.Request) {
    id, err := parseID(r, "id")
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    if _, err := h.Q.GetFood(r.Context(), id); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeError(w, http.StatusNotFound, "food not found"); return
        }
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    var body updateFoodBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    if body.CaloriesPerUnit < 0 {
        writeError(w, http.StatusBadRequest, "calories_per_unit must be >= 0"); return
    }
    if body.ProteinPerUnit < 0 {
        writeError(w, http.StatusBadRequest, "protein_per_unit must be >= 0"); return
    }

    tx, err := h.DB.BeginTx(r.Context(), nil)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    defer func() { _ = tx.Rollback() }()
    q := h.Q.WithTx(tx)

    food, err := q.UpdateFoodNutrition(r.Context(), queries.UpdateFoodNutritionParams{
        ID:              id,
        CaloriesPerUnit: body.CaloriesPerUnit,
        ProteinPerUnit:  body.ProteinPerUnit,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    foodID := id
    if err := q.RestampLogEntriesForFood(r.Context(), queries.RestampLogEntriesForFoodParams{
        CaloriesPerUnit: body.CaloriesPerUnit,
        ProteinPerUnit:  body.ProteinPerUnit,
        FoodID:          &foodID,
    }); err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    if err := tx.Commit(); err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    writeJSON(w, http.StatusOK, food)
}
```

`ListFoods` and `DeleteFood` need no change — they pass `queries.Food` straight through and the regenerated struct carries `ProteinPerUnit`.

### `backend/handlers/log.go`

`AddLogEntry` includes protein in the payload + the computed `protein` column:

```go
type addLogBody struct {
    FoodID          *int64  `json:"food_id"`
    FoodName        string  `json:"food_name"`
    FoodUnit        string  `json:"food_unit"`
    CaloriesPerUnit float64 `json:"calories_per_unit"`
    ProteinPerUnit  float64 `json:"protein_per_unit"`
    Quantity        float64 `json:"quantity"`
    Date            string  `json:"date"`
}

func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
    // … existing parse + validation …
    if body.ProteinPerUnit < 0 {
        writeError(w, http.StatusBadRequest, "protein_per_unit must be >= 0"); return
    }
    entry, err := h.Q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
        UserID:           userID,
        FoodID:           body.FoodID,
        Date:             body.Date,
        FoodName:         body.FoodName,
        FoodUnit:         body.FoodUnit,
        CaloriesPerUnit:  body.CaloriesPerUnit,
        ProteinPerUnit:   body.ProteinPerUnit,
        Quantity:         body.Quantity,
        Calories:         body.CaloriesPerUnit * body.Quantity,
        Protein:          body.ProteinPerUnit  * body.Quantity,
        SourceRecipeID:   nil,
        SourceRecipeName: nil,
    })
    // …
}
```

`GetLog`, `DeleteLogEntry`, `DeleteLogEntriesByRecipe`, `GetRecentFoods` need no handler changes.

### `backend/handlers/recipes.go`

Two changes:

1. **`LogRecipe`** — when expanding ingredients into `log_entries`, multiply the joined protein the same way calories are computed (`recipes.go:271–287` is the loop):

```go
for _, ing := range ings {
    foodID := ing.FoodID
    recipeID := recipe.ID
    recipeName := recipe.Name
    qty := ing.Quantity * body.Scale
    entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
        UserID:           userID,
        FoodID:           &foodID,
        Date:             body.Date,
        FoodName:         ing.FoodName,
        FoodUnit:         ing.FoodUnit,
        CaloriesPerUnit:  ing.CaloriesPerUnit,
        ProteinPerUnit:   ing.ProteinPerUnit, // NEW: now joined from foods
        Quantity:         qty,
        Calories:         ing.CaloriesPerUnit * qty,
        Protein:          ing.ProteinPerUnit  * qty,
        SourceRecipeID:   &recipeID,
        SourceRecipeName: &recipeName,
    })
    // …
}
```

2. **`GetRecipe`** — extend `recipeDetailResponse` and sum protein alongside calories in the in-Go aggregation:

```go
type recipeDetailResponse struct {
    ID            int64                              `json:"id"`
    Name          string                             `json:"name"`
    CreatedAt     string                             `json:"created_at"`
    TotalCalories float64                            `json:"total_calories"`
    TotalProtein  float64                            `json:"total_protein"`
    Ingredients   []queries.GetRecipeIngredientsRow  `json:"ingredients"`
}

// inside GetRecipe handler:
var totalCal, totalProt float64
for _, ing := range ings {
    totalCal  += ing.CaloriesPerUnit * ing.Quantity
    totalProt += ing.ProteinPerUnit  * ing.Quantity
}
writeJSON(w, http.StatusOK, recipeDetailResponse{
    ID: recipe.ID, Name: recipe.Name, CreatedAt: recipe.CreatedAt,
    TotalCalories: totalCal,
    TotalProtein:  totalProt,
    Ingredients:   ings,
})
```

`ListRecipes` requires no handler change — the regenerated `ListRecipesRow` carries `TotalProtein` from the SQL.

`CreateRecipe`, `UpdateRecipe`, `DeleteRecipe` need no change.

### `backend/handlers/metrics.go`

`metricsResponse` gets two new fields; `GetMetrics` switches to `SumNutritionByDateRange` and merges protein too:

```go
type metricsResponse struct {
    ID               int64    `json:"id"`
    UserID           int64    `json:"user_id"`
    Date             string   `json:"date"`
    Weight           *float64 `json:"weight"`
    Steps            *int64   `json:"steps"`
    CaloriesConsumed float64  `json:"calories_consumed"`
    ProteinConsumed  float64  `json:"protein_consumed"`
}

// inside GetMetrics:
nutritionRows, err := h.Q.SumNutritionByDateRange(r.Context(), queries.SumNutritionByDateRangeParams{
    UserID: userID, FromDate: from, ToDate: to,
})
calByDate := make(map[string]float64, len(nutritionRows))
protByDate := make(map[string]float64, len(nutritionRows))
for _, n := range nutritionRows {
    calByDate[n.Date]  = n.TotalCalories
    protByDate[n.Date] = n.TotalProtein
}
// in the per-day loop:
out = append(out, metricsResponse{
    // …existing fields…
    CaloriesConsumed: calByDate[m.Date],
    ProteinConsumed:  protByDate[m.Date],
})
```

`GetTodaySummary` returns both:

```go
writeJSON(w, http.StatusOK, map[string]float64{
    "consumed":         row.Consumed,
    "target":           float64(row.Target),
    "protein_consumed": row.ProteinConsumed,
    "target_protein":   float64(row.TargetProtein),
})
```

`UpsertMetrics` unchanged.

### `backend/handlers/users.go`

`CreateUser` and `UpdateUser` accept `target_protein`. Validation: `>= 0` (allow zero — represents "not tracking protein"; UI shows the same "no target set" affordance as target_calories does today on `Home.vue:110`).

```go
type createUserBody struct {
    Name           string `json:"name"`
    Avatar         string `json:"avatar"`
    TargetCalories int64  `json:"target_calories"`
    TargetProtein  int64  `json:"target_protein"`
}

// in CreateUser, after the existing target_calories check:
if body.TargetProtein < 0 {
    writeError(w, http.StatusBadRequest, "target_protein must be >= 0"); return
}
u, err := h.Q.CreateUser(r.Context(), queries.CreateUserParams{
    Name:           name,
    Avatar:         avatar,
    TargetCalories: body.TargetCalories,
    TargetProtein:  body.TargetProtein,
})

// updateUserBody and UpdateUser mirror the same:
type updateUserBody struct {
    Name           *string `json:"name"`
    TargetCalories *int64  `json:"target_calories"`
    TargetProtein  *int64  `json:"target_protein"`
}
// then a parallel block to the existing target_calories one
```

### `backend/handlers/ai.go`

Extend the prompt so the hint mentions both. The response is still a single free-text `hint` string — no shape change.

```go
prompt := fmt.Sprintf(
    "ANSWER IN ONE LINE: how much calories and protein (g) in %s ?",
    req.Name,
)
```

### `backend/cmd/seed/main.go`

Add protein values to the seed foods so the seeded data exercises the new column. Suggested values (g/unit):

```go
foods := []seedFood{
    {"Oatmeal",         "g",     3.9,  0.13},
    {"Banana",          "piece", 89,   1.1},
    {"Grilled Chicken", "g",     1.65, 0.31},
    {"Brown Rice",      "g",     1.3,  0.026},
    {"Almonds",         "g",     5.8,  0.21},
    {"Greek Yogurt",    "ml",    0.67, 0.10},
    {"Olive Oil",       "ml",    8.8,  0.0},
    {"Egg",             "piece", 78,   6.3},
    {"Avocado",         "g",     1.6,  0.02},
    {"Salmon",          "g",     2.08, 0.20},
}
// add the protein arg to seedFood struct + CreateFoodParams
```

Seed users also get a `TargetProtein`:

```go
users := []queries.CreateUserParams{
    {Name: "Mohit", Avatar: "MO", TargetCalories: 2200, TargetProtein: 140},
    {Name: "Sara",  Avatar: "SR", TargetCalories: 1800, TargetProtein: 100},
}
```

In the `AddLogEntry` loop inside the seed, pass `ProteinPerUnit: f.ProteinPerUnit` and `Protein: f.ProteinPerUnit * qty` (mirrors how calories are computed).

---

## Phase 4 — Frontend types + API client

### `frontend/src/lib/types.ts`

```ts
export interface User {
  id: number
  name: string
  avatar: string
  target_calories: number
  target_protein: number
  created_at: string
}

export interface Food {
  id: number
  name: string
  unit: string
  calories_per_unit: number
  protein_per_unit: number
  created_at: string
}

export interface LogEntry {
  id: number
  user_id: number
  food_id: number | null
  date: string
  food_name: string
  food_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: number
  calories: number
  protein: number
  source_recipe_id: number | null
  source_recipe_name: string | null
}

export interface DailyMetric {
  id: number
  user_id: number
  date: string
  weight: number | null
  steps: number | null
  calories_consumed: number
  protein_consumed: number
}

export interface TodaySummary {
  consumed: number
  target: number
  protein_consumed: number
  target_protein: number
}

export interface CreateFoodPayload {
  name: string
  unit: string
  calories_per_unit: number
  protein_per_unit: number
}

export interface UpdateFoodPayload {
  calories_per_unit: number
  protein_per_unit: number
}

export interface CreateUserPayload {
  name: string
  target_calories: number
  target_protein: number
}

export interface UpdateUserPayload {
  name?: string
  target_calories?: number
  target_protein?: number
}

export interface AddLogPayload {
  food_id: number | null
  food_name: string
  food_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: number
  date: string
}

export interface RecipeIngredient {
  id: number
  recipe_id: number
  food_id: number
  quantity: number
  food_name: string
  food_unit: string
  calories_per_unit: number
  protein_per_unit: number
}

export interface RecipeListItem extends Recipe {
  total_calories: number
  total_protein: number
}

export interface RecipeWithIngredients extends RecipeListItem {
  ingredients: RecipeIngredient[]
}

// RecentFood: no change — protein not surfaced in the recent list.
```

### `frontend/src/lib/api.ts`

No structural changes — JSON pass-through. Only the type imports above need to exist.

---

## Phase 5 — Frontend UI

### `frontend/src/components/FoodEditor.vue`

Add a protein input field, parallel to calories. Edit mode keeps name+unit read-only but **both** calories and protein become editable.

```vue
<script setup lang="ts">
// existing refs plus:
const protein = ref<string>('')

function reset(): void {
  name.value = props.food?.name ?? ''
  unit.value = props.food?.unit ?? 'g'
  calories.value = props.food != null ? String(props.food.calories_per_unit) : ''
  protein.value  = props.food != null ? String(props.food.protein_per_unit)  : ''
  // …
}

async function save(): Promise<void> {
  const cal  = Number(calories.value)
  const prot = Number(protein.value)
  if (!Number.isFinite(cal)  || cal  < 0) { errMsg.value = 'Enter a non-negative calorie value'; return }
  if (!Number.isFinite(prot) || prot < 0) { errMsg.value = 'Enter a non-negative protein value'; return }
  if (!isEdit.value && !name.value.trim()) { errMsg.value = 'Name required'; return }

  saving.value = true; errMsg.value = ''
  try {
    if (props.food != null) {
      await api.updateFood(props.food.id, { calories_per_unit: cal, protein_per_unit: prot })
    } else {
      await api.createFood({
        name: name.value.trim(),
        unit: unit.value,
        calories_per_unit: cal,
        protein_per_unit:  prot,
      })
    }
    emit('saved'); emit('update:open', false)
  } catch (e) { /* … */ }
  finally { saving.value = false }
}
</script>
```

Template: the existing calorie+unit row stays; below it add a protein row. The "Calories per 1 {unit}" caption gets a parallel "Protein (g) per 1 {unit}" caption:

```vue
<div class="flex gap-2">
  <Input
    v-model="protein"
    type="number"
    inputmode="decimal"
    placeholder="Protein"
    min="0"
    step="0.1"
  />
  <div class="h-9 rounded-md border border-input bg-muted px-3 text-sm grid place-items-center text-muted-foreground">
    g
  </div>
</div>
<p class="text-xs text-muted-foreground">Protein (g) per 1 {{ unit }}</p>
```

AI hint: no change to the call site — the backend already extends the prompt. The hint copy will mention both kcal and protein naturally.

### `frontend/src/components/AddFoodDrawer.vue`

Two extensions:

1. `PickedFood` carries `protein_per_unit`; `PickedRecipe` carries `total_protein`.
2. The live preview line shows protein alongside calories.

```ts
interface PickedFood {
  kind: 'food'
  food_id: number | null
  food_name: string
  food_unit: string
  calories_per_unit: number
  protein_per_unit: number  // NEW
  quantity: string
}

interface PickedRecipe {
  kind: 'recipe'
  recipe_id: number
  recipe_name: string
  total_calories: number
  total_protein: number      // NEW
  scale: string
}

const previewCalories = computed<number>(() => /* unchanged */)
const previewProtein  = computed<number>(() => {
  if (!picked.value) return 0
  const n = Number(picked.value.kind === 'food' ? picked.value.quantity : picked.value.scale)
  if (!Number.isFinite(n) || n <= 0) return 0
  if (picked.value.kind === 'food') return n * picked.value.protein_per_unit
  return n * picked.value.total_protein
})
```

`pickRecent`, `pickLibraryFood`, `pickLibraryRecipe` each copy the new field from their source. `confirm()` sends `protein_per_unit` in the `AddLogPayload`. `RecentFood` doesn't include protein yet — pick from foods/recipes search to ensure protein is correct; for recent picks, leave `protein_per_unit: 0` is acceptable since the recent list already only stores a `calories_per_unit` snapshot (`log.sql:7–22`). **Better**: extend `GetRecentLoggedFoods` to also return `protein_per_unit` so recent picks carry protein too — see "Adjustment to recent foods" below.

#### Adjustment to recent foods (small follow-on)

To keep recent-pick protein accurate, extend `GetRecentLoggedFoods` to also return `protein_per_unit`:

```sql
-- name: GetRecentLoggedFoods :many
SELECT
  le.food_name,
  le.food_unit,
  le.calories_per_unit,
  le.protein_per_unit,
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
```

Then `RecentFood` in `types.ts` gains `protein_per_unit: number`, and `pickRecent` copies it. No UI surface change — the list rows stay calories-only, but the picked food carries the correct protein into the confirmation step.

Template preview line:

```vue
<span v-if="previewCalories > 0" class="text-sm ml-auto">
  ≈ {{ Math.round(previewCalories) }} kcal · {{ Math.round(previewProtein) }} g protein
</span>
```

### `frontend/src/components/RecipeEditor.vue`

Add a `totalProtein` computed alongside `totalCalories`, and surface it in the footer panel:

```ts
const totalProtein = computed<number>(() => {
  let total = 0
  for (const ing of ingredients.value) {
    const qty = Number(ing.quantity)
    if (Number.isFinite(qty) && qty > 0) {
      total += qty * ing.protein_per_unit
    }
  }
  return total
})
```

`DraftIngredient` needs `protein_per_unit: number`; `addIngredient` copies it from the `Food` lookup; `loadForEdit` maps from `ingredients[i].protein_per_unit`. The ingredient row also displays it inline:

```vue
<div class="text-xs text-muted-foreground">
  {{ ing.calories_per_unit }} kcal · {{ ing.protein_per_unit }} g protein / 1 {{ ing.food_unit }}
</div>
```

Footer:

```vue
<div class="rounded-md bg-muted px-3 py-2 flex items-center justify-between">
  <span class="text-xs text-muted-foreground uppercase tracking-wide">Per serving</span>
  <span class="font-semibold">
    {{ Math.round(totalCalories) }} kcal · {{ Math.round(totalProtein) }} g protein
  </span>
</div>
```

### `frontend/src/views/FoodLibrary.vue`

Food list row: extend the kcal line to include protein.

```vue
<div class="text-xs text-muted-foreground">
  {{ f.calories_per_unit }} kcal · {{ f.protein_per_unit }} g protein / 1 {{ f.unit }}
</div>
```

Recipe list row gets protein too:

```vue
<div class="text-xs text-muted-foreground">
  {{ Math.round(r.total_calories) }} kcal · {{ Math.round(r.total_protein) }} g protein / serving
</div>
```

### `frontend/src/views/UserPage.vue`

Three changes:

1. **Log table** — add a `Protein` column (between Cal and the trash). Update `LogGroup` type so the recipe header still aggregates protein.

```ts
type LogGroup =
  | { kind: 'single'; entry: LogEntry }
  | {
      kind: 'recipe'
      recipeId: number
      recipeName: string
      entries: LogEntry[]
      totalCalories: number
      totalProtein: number
    }

// in the groups computed, when finalizing recipe groups:
out.push({
  kind: 'recipe',
  recipeId: item.recipeId,
  recipeName: arr[0].source_recipe_name ?? 'Recipe',
  entries: arr,
  totalCalories: arr.reduce((s, e) => s + e.calories, 0),
  totalProtein:  arr.reduce((s, e) => s + e.protein,  0),
})

const totalCalories = computed(() =>
  Math.round(entries.value.reduce((s, e) => s + e.calories, 0))
)
const totalProtein = computed(() =>
  Math.round(entries.value.reduce((s, e) => s + e.protein, 0))
)
```

Table header row:

```vue
<tr class="border-b border-border">
  <th class="text-left  font-medium px-3 py-2">Food</th>
  <th class="text-right font-medium px-3 py-2 w-16">Qty</th>
  <th class="text-right font-medium px-3 py-2 w-16">Cal</th>
  <th class="text-right font-medium px-3 py-2 w-16">Prot</th>
  <th class="w-9" />
</tr>
```

Single row gets the new cell:

```vue
<td class="text-right px-3 py-2">{{ Math.round(g.entry.protein) }}</td>
```

Recipe header row totals both:

```vue
<td class="text-right px-3 py-2 font-medium">{{ Math.round(g.totalCalories) }}</td>
<td class="text-right px-3 py-2 font-medium">{{ Math.round(g.totalProtein)  }}</td>
```

Indented ingredient rows show their individual protein in muted text. Footer row:

```vue
<div class="px-3 py-3 border-t border-border flex justify-between font-semibold">
  <span>Total</span>
  <span>
    <span class="text-[hsl(var(--chart-blue))]">{{ formatNumber(totalCalories) }} kcal</span>
    <span class="ml-3 text-[hsl(var(--chart-violet))]">{{ formatNumber(totalProtein) }} g</span>
  </span>
</div>
```

2. **Target protein input** under the existing Weight / Steps / Target calories card. Same debounced-save-on-blur pattern as target calories (`UserPage.vue:171–188`):

```ts
const targetProtein = ref<string>('')
let savingTargetProtein = false

function syncTargetFromUser(): void {
  const u = user.value
  targetCalories.value = u ? String(u.target_calories) : ''
  targetProtein.value  = u ? String(u.target_protein)  : ''
}

async function saveTargetProtein() {
  if (savingTargetProtein) return
  const t = parseOptionalNumber(targetProtein.value)
  if (t == null || t < 0) { syncTargetFromUser(); return }
  const rounded = Math.round(t)
  if (user.value && user.value.target_protein === rounded) return
  savingTargetProtein = true
  try {
    const updated = await api.updateUser(props.userId, { target_protein: rounded })
    userStore.upsert(updated)
    targetProtein.value = String(updated.target_protein)
  } finally {
    savingTargetProtein = false
  }
}
```

Template (after the Target calories row):

```vue
<div class="flex items-center gap-3">
  <label class="w-32 text-sm text-muted-foreground">Target protein</label>
  <Input
    v-model="targetProtein"
    type="number"
    inputmode="numeric"
    min="0"
    step="5"
    placeholder="0"
    @blur="saveTargetProtein"
  />
  <span class="text-xs text-muted-foreground w-10">g</span>
</div>
```

3. **Pass protein target to `ProgressCharts`**:

```vue
<ProgressCharts
  v-else
  :data="metricsRange"
  :target="user?.target_calories ?? 0"
  :protein-target="user?.target_protein ?? 0"
/>
```

### `frontend/src/views/Home.vue`

New-user dialog accepts target protein:

```ts
const newProtein = ref<string>('0')

async function submitAdd() {
  const name = newName.value.trim()
  const t = Number(newTarget.value)
  const p = Number(newProtein.value)
  if (!name) return
  if (!Number.isFinite(t) || t <= 0) {
    errMsg.value = 'Daily calorie target must be a positive number'; return
  }
  if (!Number.isFinite(p) || p < 0) {
    errMsg.value = 'Daily protein target must be >= 0'; return
  }
  adding.value = true; errMsg.value = ''
  try {
    const u = await userStore.add({
      name,
      target_calories: Math.round(t),
      target_protein:  Math.round(p),
    })
    // …
  } /* … */
}
```

Template input (after the calorie target):

```vue
<div class="flex flex-col gap-1">
  <Input
    v-model="newProtein"
    type="number"
    inputmode="numeric"
    min="0"
    step="5"
    placeholder="Daily protein target (g)"
    @keyup.enter="submitAdd"
  />
  <p class="text-xs text-muted-foreground">Daily protein target (g) — 0 if not tracking</p>
</div>
```

No donut change on Home — the existing kcal donut stays as the headline.

### `frontend/src/components/ProgressCharts.vue`

Add a `proteinTarget` prop and a new line chart card between Calories and Weight.

```ts
const props = defineProps<{ data: DailyMetric[]; target: number; proteinTarget: number }>()

const proteinData = computed<ChartData<'line'>>(() => ({
  labels: labels.value,
  datasets: [
    {
      label: 'Consumed',
      data: props.data.map((d) => Math.round(d.protein_consumed)),
      borderColor: violet.value,
      backgroundColor: violet.value,
      tension: 0.35,
      pointRadius: 3,
      fill: false,
    },
    {
      label: 'Target',
      data: props.data.map(() => (props.proteinTarget > 0 ? props.proteinTarget : null)),
      borderColor: amber.value,
      backgroundColor: amber.value,
      borderDash: [6, 4],
      tension: 0,
      pointRadius: 0,
      fill: false,
    },
  ],
}))
```

Template (insert between the Calories card and the Weight card):

```vue
<Card>
  <div class="p-4 pb-2">
    <div class="text-sm font-semibold">Protein</div>
  </div>
  <div class="px-3 pb-3">
    <div class="h-40">
      <Line v-if="hasData" :data="proteinData" :options="lineOpts" />
      <div v-else class="h-full grid place-items-center text-muted-foreground text-sm">
        Not enough data
      </div>
    </div>
  </div>
</Card>
```

Reuses `lineOpts` and the `violet`/`amber` chart colours that are already defined.

---

## Phase 6 — Testing checklist

Backend
- [ ] `POST /api/foods` with `{name, unit, calories_per_unit, protein_per_unit}` → 201, returns row with both.
- [ ] `POST /api/foods` with `protein_per_unit < 0` → 400.
- [ ] `PUT /api/foods/{id}` with `{calories_per_unit, protein_per_unit}` → 200; every existing `log_entries` row with that `food_id` now has the new `protein_per_unit` and `protein = new_pp × quantity` **and** the new `calories_per_unit` and `calories = new_cpu × quantity`. Both updates ride one transaction.
- [ ] Log entries with `food_id IS NULL` are not restamped.
- [ ] `POST /api/users` with `{name, target_calories, target_protein}` → 201.
- [ ] `POST /api/users` with `target_protein < 0` → 400.
- [ ] `POST /api/users` with `target_protein = 0` succeeds (allowed; means "not tracking").
- [ ] `PUT /api/users/{id}` with `{target_protein: 130}` → 200.
- [ ] `POST /api/users/{id}/log` with `{...protein_per_unit, quantity}` → 201, `protein` column is computed server-side as `protein_per_unit × quantity`.
- [ ] `POST /api/users/{id}/log/recipe` expands a recipe; every resulting `log_entries` row has the correct protein computed from the (live) food protein × `quantity × scale`.
- [ ] `GET /api/users/{id}/today` returns `{consumed, target, protein_consumed, target_protein}`.
- [ ] `GET /api/users/{id}/metrics?from=…&to=…` returns `protein_consumed` per day, summed correctly across single-food + recipe-sourced entries.
- [ ] `GET /api/recipes` carries `total_protein` per row; `GET /api/recipes/{id}` carries `total_protein` and per-ingredient `protein_per_unit`.
- [ ] Edit a food's protein → recipe totals update on next read (live join).
- [ ] AI hint: `POST /api/ai/calorie-hint` with `{name: "banana"}` → response includes a one-line mention of both kcal and protein.
- [ ] Legacy DB (one without the new columns): startup runs the `ensureColumn` migrations, columns appear with default `0`, no rows lost.

Frontend
- [ ] `vue-tsc -b --force` clean.
- [ ] FoodEditor (create): inputs accept name, unit, calories, protein; AI hint shows both. Save calls `createFood` with all four fields.
- [ ] FoodEditor (edit): name+unit read-only; calories AND protein both editable. Save updates both.
- [ ] FoodLibrary food row shows `kcal · g protein / 1 unit`.
- [ ] FoodLibrary recipe row shows `kcal · g protein / serving`.
- [ ] RecipeEditor footer shows `Per serving: X kcal · Y g protein`.
- [ ] AddFoodDrawer preview shows `≈ N kcal · M g protein`.
- [ ] AddFoodDrawer recent pick: the picked-row carries correct protein (after the `GetRecentLoggedFoods` extension).
- [ ] UserPage log table has a Protein column; totals row shows both kcal and g.
- [ ] UserPage Target protein input persists on blur; reloads to the saved value.
- [ ] Home dialog accepts target protein on new user; persists.
- [ ] ProgressCharts shows four cards in order (Calories, Protein, Weight, Steps); the Protein card title reads `Protein` and the dashed target line is visible only when `target_protein > 0`.
- [ ] Empty-data state in protein chart renders "Not enough data" identical to calories.

End-to-end smoke
- [ ] Create a food with kcal=2, protein=0.3 → log 100 of it → log row shows 200 kcal, 30 g protein → today summary reflects both → progress chart points for that day match → edit the food to protein=0.5 → existing log row shows 50 g protein, today summary reflects 50 g, chart point updates.
- [ ] Create a recipe using that food (qty=50) and another (qty=20) → recipe footer shows correct totals; log the recipe with scale=2 → two log rows appear, both carrying correct protein; recipe-group header sums them.

---

## Phase 7 — Rollout order

1. **Schema + ensureColumn migrations** (both schema files; `db.go`). Verify on a copy of an existing DB.
2. **sqlc queries** — extend `foods.sql`, `log.sql`, `recipes.sql`, `metrics.sql`, `users.sql`. Run `sqlc generate`.
3. **Backend handlers** — `foods.go`, `log.go`, `recipes.go` (`LogRecipe` and `GetRecipe`), `metrics.go`, `users.go`, `ai.go` (prompt only). `go build ./...` clean.
4. **Seed script** — add protein values so a fresh `cd backend && go run ./cmd/seed` produces realistic data.
5. **Frontend types + API client** — `types.ts` interfaces (`api.ts` is JSON pass-through, no real change).
6. **Frontend UI** — in this order, each verifiable independently:
   1. `FoodEditor.vue` + `FoodLibrary.vue` (create/edit/list show protein).
   2. `RecipeEditor.vue` (per-serving totals).
   3. `AddFoodDrawer.vue` (preview + recent shortcut).
   4. `UserPage.vue` (log column, target protein input, prop pass-through).
   5. `Home.vue` (new-user dialog).
   6. `ProgressCharts.vue` (new chart).
7. **Manual smoke test** — see end-to-end checklist above.
8. **Update `research.md`** — `features/add-protein/research.md` was written before this work; once shipped, add a short "Implemented" note at the top of §9 so the next reader knows the protein column is no longer hypothetical.

---

## Open considerations

- **`UpdateFood` becomes "update nutrition", not "update calories".** Renaming the sqlc query (`UpdateFoodCalories` → `UpdateFoodNutrition`) keeps the code honest. The HTTP route stays `PUT /api/foods/{id}`; the body now carries two fields. Single-tenant app, no API consumers to break.
- **Allowing `target_protein = 0`** lets users opt out of protein tracking without removing the column. The UI degrades gracefully because the protein chart's target dashed line is conditional on `proteinTarget > 0` (same trick as the calories target line).
- **`GetRecentLoggedFoods` widening** is the only place where adding the protein column would be technically optional — the recent UI doesn't show it — but skipping the widening means a recent-pick re-log would default to 0 g protein and silently understate the user's intake. Wider is safer.
- **No structured AI response.** A future enhancement could parse Gemini's reply to pre-fill the calorie and protein inputs. Out of scope here — the user just reads the text and types numbers.
- **Audit trail of edits.** Still not tracked, same as today's calorie edits. If audit matters later, the same `food_revisions` table idea in `plans/editable-food.md:432` would now record both kcal and protein.
