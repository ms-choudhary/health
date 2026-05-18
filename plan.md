# Recipes Implementation Plan

## Goals

Add support for **recipes** (a.k.a. macros / composite foods) such as "Mango Shake" that consist of multiple simple food items.

### Design rules

1. **Simple food** stays as today: `name`, `unit`, `calories_per_unit`.
2. **Recipe** = `name` + list of `(food_id, quantity)` ingredients. No `calories_per_unit` stored — derived.
3. **One recipe = one serving.** Logging quantity is a **scale factor** (1 = the listed amounts, 0.5 = half, 2 = double).
4. **Recipes are never logged directly.** On log, the recipe is decomposed into N simple `log_entries` (one per ingredient, with quantity = `ingredient_quantity × scale_factor`).
5. **Log entries are tagged** with `source_recipe_id` and `source_recipe_name` so the UI can group and "delete-as-group."
6. **No nested recipes.** Ingredients reference `foods` only.
7. **No AI hint** for recipes.

### Property we get for free

Because the decomposition happens at log time and uses the existing snapshot pattern, editing or deleting a recipe **never affects historical logs**.

---

## Phase 1 — Database schema

### New tables (additions to `backend/sql/schema.sql`)

```sql
CREATE TABLE IF NOT EXISTS recipes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS recipe_ingredients (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  food_id   INTEGER NOT NULL REFERENCES foods(id)   ON DELETE RESTRICT,
  quantity  REAL    NOT NULL CHECK(quantity > 0)
);

CREATE INDEX IF NOT EXISTS idx_recipe_ingredients_recipe ON recipe_ingredients(recipe_id);
```

Notes:
- `ON DELETE RESTRICT` on `food_id` prevents deleting a food that is still used by a recipe (forces the user to fix the recipe first — saner than silent NULLs).
- `ON DELETE CASCADE` on `recipe_id` cleans up ingredients when a recipe is deleted.

### Migration on `log_entries`

Add two nullable columns:

```sql
ALTER TABLE log_entries ADD COLUMN source_recipe_id   INTEGER;
ALTER TABLE log_entries ADD COLUMN source_recipe_name TEXT;
```

SQLite has no `IF NOT EXISTS` for `ADD COLUMN`, so update `backend/db/db.go` with a tiny idempotent helper that runs after `schema.sql`:

```go
// ensureColumn adds a column if it doesn't already exist on the given table.
func ensureColumn(conn *sql.DB, table, column, definition string) error {
    rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
    if err != nil {
        return err
    }
    defer rows.Close()
    for rows.Next() {
        var cid int
        var name, ctype string
        var notnull, pk int
        var dflt sql.NullString
        if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
            return err
        }
        if name == column {
            return nil
        }
    }
    _, err = conn.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
    return err
}
```

Call from `Init()`:

```go
if err := ensureColumn(conn, "log_entries", "source_recipe_id",   "INTEGER"); err != nil { return nil, err }
if err := ensureColumn(conn, "log_entries", "source_recipe_name", "TEXT");    return nil, err }
```

---

## Phase 2 — Backend SQL (sqlc)

New file `backend/sql/queries/recipes.sql`:

```sql
-- name: CreateRecipe :one
INSERT INTO recipes (name) VALUES (?) RETURNING *;

-- name: DeleteRecipe :exec
DELETE FROM recipes WHERE id = ?;

-- name: GetRecipe :one
SELECT * FROM recipes WHERE id = ?;

-- name: ListRecipes :many
SELECT
  r.id,
  r.name,
  r.created_at,
  CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories
FROM recipes r
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN foods f               ON f.id          = ri.food_id
GROUP BY r.id
ORDER BY r.name;

-- name: SearchRecipes :many
SELECT
  r.id,
  r.name,
  r.created_at,
  CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories
FROM recipes r
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN foods f               ON f.id          = ri.food_id
WHERE r.name LIKE '%' || ?1 || '%'
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
  f.calories_per_unit AS calories_per_unit
FROM recipe_ingredients ri
JOIN foods f ON f.id = ri.food_id
WHERE ri.recipe_id = ?
ORDER BY ri.id;

-- name: AddRecipeIngredient :one
INSERT INTO recipe_ingredients (recipe_id, food_id, quantity)
VALUES (?, ?, ?)
RETURNING *;

-- name: DeleteRecipeIngredient :exec
DELETE FROM recipe_ingredients WHERE id = ? AND recipe_id = ?;

-- name: ClearRecipeIngredients :exec
DELETE FROM recipe_ingredients WHERE recipe_id = ?;
```

Update `backend/sql/queries/log.sql`:

```sql
-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, food_id, date, food_name, food_unit, calories_per_unit, quantity, calories,
   source_recipe_id, source_recipe_name)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLogEntriesByRecipe :exec
DELETE FROM log_entries
WHERE user_id = ?1 AND date = ?2 AND source_recipe_id = ?3;
```

Run `sqlc generate` to regenerate `backend/db/queries/`.

---

## Phase 3 — Backend handlers

### New file `backend/handlers/recipes.go`

Routes:

| Method | Path | Purpose |
|--------|------|---------|
| GET    | `/api/recipes` | List recipes (with `total_calories`); `?q=` searches by name |
| POST   | `/api/recipes` | Create recipe (name + initial ingredients) |
| GET    | `/api/recipes/{id}` | Get recipe with ingredients |
| PUT    | `/api/recipes/{id}` | Replace name + ingredients atomically |
| DELETE | `/api/recipes/{id}` | Delete recipe (ingredients cascade) |

Atomic create/update — replace whole ingredient list in a single transaction:

```go
type recipeBody struct {
    Name        string             `json:"name"`
    Ingredients []ingredientInput  `json:"ingredients"`
}
type ingredientInput struct {
    FoodID   int64   `json:"food_id"`
    Quantity float64 `json:"quantity"`
}

func (h *Handler) CreateRecipe(w http.ResponseWriter, r *http.Request) {
    var body recipeBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    body.Name = strings.TrimSpace(body.Name)
    if body.Name == "" {
        writeError(w, http.StatusBadRequest, "name required"); return
    }
    for _, ing := range body.Ingredients {
        if ing.FoodID <= 0 || ing.Quantity <= 0 {
            writeError(w, http.StatusBadRequest, "invalid ingredient"); return
        }
    }

    tx, err := h.DB.BeginTx(r.Context(), nil)
    if err != nil { writeError(w, 500, err.Error()); return }
    defer tx.Rollback()
    q := h.Q.WithTx(tx)

    recipe, err := q.CreateRecipe(r.Context(), body.Name)
    if err != nil { writeError(w, 500, err.Error()); return }

    for _, ing := range body.Ingredients {
        if _, err := q.AddRecipeIngredient(r.Context(), queries.AddRecipeIngredientParams{
            RecipeID: recipe.ID, FoodID: ing.FoodID, Quantity: ing.Quantity,
        }); err != nil {
            writeError(w, 500, err.Error()); return
        }
    }
    if err := tx.Commit(); err != nil { writeError(w, 500, err.Error()); return }

    writeJSON(w, http.StatusCreated, recipe)
}
```

`UpdateRecipe` is the same but reuses an existing `id`:
1. Update name (add `UpdateRecipeName` query, trivial).
2. `ClearRecipeIngredients` for that recipe.
3. Re-insert from body.

All inside one transaction.

### Logging a recipe — new endpoint

`POST /api/users/{id}/log/recipe`

Body:

```json
{
  "recipe_id": 7,
  "scale": 1.0,
  "date": "2026-05-17"
}
```

Handler:

```go
type logRecipeBody struct {
    RecipeID int64   `json:"recipe_id"`
    Scale    float64 `json:"scale"`
    Date     string  `json:"date"`
}

func (h *Handler) LogRecipe(w http.ResponseWriter, r *http.Request) {
    userID, err := parseID(r, "id")
    if err != nil { writeError(w, 400, err.Error()); return }
    var body logRecipeBody
    if err := readJSON(r, &body); err != nil { writeError(w, 400, err.Error()); return }
    if body.RecipeID <= 0 || body.Scale <= 0 || !validDate(body.Date) {
        writeError(w, 400, "invalid request"); return
    }

    recipe, err := h.Q.GetRecipe(r.Context(), body.RecipeID)
    if err != nil { writeError(w, 404, "recipe not found"); return }

    ings, err := h.Q.GetRecipeIngredients(r.Context(), body.RecipeID)
    if err != nil { writeError(w, 500, err.Error()); return }
    if len(ings) == 0 {
        writeError(w, 400, "recipe has no ingredients"); return
    }

    tx, err := h.DB.BeginTx(r.Context(), nil)
    if err != nil { writeError(w, 500, err.Error()); return }
    defer tx.Rollback()
    q := h.Q.WithTx(tx)

    out := make([]queries.LogEntry, 0, len(ings))
    for _, ing := range ings {
        qty := ing.Quantity * body.Scale
        entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
            UserID:           userID,
            FoodID:           &ing.FoodID,
            Date:             body.Date,
            FoodName:         ing.FoodName,
            FoodUnit:         ing.FoodUnit,
            CaloriesPerUnit:  ing.CaloriesPerUnit,
            Quantity:         qty,
            Calories:         ing.CaloriesPerUnit * qty,
            SourceRecipeID:   &recipe.ID,
            SourceRecipeName: &recipe.Name,
        })
        if err != nil { writeError(w, 500, err.Error()); return }
        out = append(out, entry)
    }
    if err := tx.Commit(); err != nil { writeError(w, 500, err.Error()); return }

    writeJSON(w, http.StatusCreated, out)
}
```

### Delete a recipe-group from the log

Reuse the existing `DELETE /api/users/{id}/log/{eid}` only for single rows. For recipe groups, add:

`DELETE /api/users/{id}/log/recipe?date=YYYY-MM-DD&source_recipe_id=N`

```go
func (h *Handler) DeleteLogEntriesByRecipe(w http.ResponseWriter, r *http.Request) {
    userID, _ := parseID(r, "id")
    date := r.URL.Query().Get("date")
    srid, _ := strconv.ParseInt(r.URL.Query().Get("source_recipe_id"), 10, 64)
    if !validDate(date) || srid <= 0 {
        writeError(w, 400, "invalid query"); return
    }
    if err := h.Q.DeleteLogEntriesByRecipe(r.Context(), queries.DeleteLogEntriesByRecipeParams{
        UserID:         userID,
        Date:           date,
        SourceRecipeID: &srid,
    }); err != nil { writeError(w, 500, err.Error()); return }
    w.WriteHeader(http.StatusNoContent)
}
```

### Route registration in `main.go`

```go
mux.HandleFunc("GET    /api/recipes",            h.ListRecipes)
mux.HandleFunc("POST   /api/recipes",            h.CreateRecipe)
mux.HandleFunc("GET    /api/recipes/{id}",       h.GetRecipe)
mux.HandleFunc("PUT    /api/recipes/{id}",       h.UpdateRecipe)
mux.HandleFunc("DELETE /api/recipes/{id}",       h.DeleteRecipe)
mux.HandleFunc("POST   /api/users/{id}/log/recipe",   h.LogRecipe)
mux.HandleFunc("DELETE /api/users/{id}/log/recipe",   h.DeleteLogEntriesByRecipe)
```

Also expose the existing food/recipe search merge if desired (see Phase 4 — easier to do client-side: two parallel calls).

---

## Phase 4 — Frontend types & API client

### `frontend/src/lib/types.ts` — additions

```ts
export interface Recipe {
  id: number;
  name: string;
  created_at: string;
}

export interface RecipeListItem extends Recipe {
  total_calories: number;
}

export interface RecipeIngredient {
  id: number;
  recipe_id: number;
  food_id: number;
  quantity: number;
  food_name: string;
  food_unit: string;
  calories_per_unit: number;
}

export interface RecipeWithIngredients extends Recipe {
  ingredients: RecipeIngredient[];
  total_calories: number;
}

// Search-picker discriminated union
export type Pickable =
  | { type: 'food';   food: Food }
  | { type: 'recipe'; recipe: RecipeListItem };

// Extend LogEntry
export interface LogEntry {
  // ...existing
  source_recipe_id?: number | null;
  source_recipe_name?: string | null;
}
```

### `frontend/src/lib/api.ts` — additions

```ts
export const recipesApi = {
  list: (q = '') =>
    get<RecipeListItem[]>(`/api/recipes${q ? `?q=${encodeURIComponent(q)}` : ''}`),

  get: (id: number) =>
    get<RecipeWithIngredients>(`/api/recipes/${id}`),

  create: (body: { name: string; ingredients: { food_id: number; quantity: number }[] }) =>
    post<Recipe>('/api/recipes', body),

  update: (id: number, body: { name: string; ingredients: { food_id: number; quantity: number }[] }) =>
    put<Recipe>(`/api/recipes/${id}`, body),

  remove: (id: number) =>
    del(`/api/recipes/${id}`),
};

export const logApi = {
  // ...existing
  logRecipe: (userId: number, body: { recipe_id: number; scale: number; date: string }) =>
    post<LogEntry[]>(`/api/users/${userId}/log/recipe`, body),

  deleteRecipeGroup: (userId: number, date: string, sourceRecipeId: number) =>
    del(`/api/users/${userId}/log/recipe?date=${date}&source_recipe_id=${sourceRecipeId}`),
};
```

---

## Phase 5 — Frontend UI

### A. Recipe management

Extend `FoodLibrary.vue` with a **Recipes** tab (or split into two child routes `/library/foods` and `/library/recipes`). The existing food management UI stays put.

**Recipe list view:** name + total calories per serving + edit/delete buttons.

**Recipe editor** (`RecipeEditorDialog.vue`, new component):

```vue
<script setup lang="ts">
const name = ref('');
const ingredients = ref<{ food_id: number; food_name: string; food_unit: string; quantity: number; cpu: number }[]>([]);

const totalCalories = computed(() =>
  ingredients.value.reduce((s, i) => s + i.cpu * i.quantity, 0)
);

async function addIngredient(food: Food, quantity: number) {
  ingredients.value.push({
    food_id: food.id,
    food_name: food.name,
    food_unit: food.unit,
    cpu: food.calories_per_unit,
    quantity,
  });
}

async function save() {
  await recipesApi.create({
    name: name.value.trim(),
    ingredients: ingredients.value.map(i => ({ food_id: i.food_id, quantity: i.quantity })),
  });
  emit('saved');
}
</script>
```

UI flow:
1. Enter recipe name.
2. Search foods (reuses existing food-picker), set quantity for each.
3. Live total calories display at the bottom.
4. Save → POST `/api/recipes`.

### B. `AddFoodDrawer.vue` — combined picker

Today it shows recent foods + library search. Change in two places:

**Search box**: fire both calls in parallel and merge results.

```ts
const results = ref<Pickable[]>([]);

watchDebounced(query, async (q) => {
  if (!q) { results.value = []; return; }
  const [foods, recipes] = await Promise.all([
    foodsApi.list(q),
    recipesApi.list(q),
  ]);
  results.value = [
    ...recipes.map(r => ({ type: 'recipe' as const, recipe: r })),
    ...foods.map(f   => ({ type: 'food'   as const, food:   f })),
  ];
}, { debounce: 200 });
```

Render with a small "Recipe" badge for recipe rows.

**On select:**
- Food → existing flow (quantity in the food's unit).
- Recipe → quantity input is the **scale factor**, default `1`. Show the recipe's total calories × scale as a live preview. Call `logApi.logRecipe()`.

### C. Log table — group recipe entries

In `UserPage.vue`, when rendering the log table, group consecutive rows by `source_recipe_id` (or compute groups in a pass):

```ts
type LogGroup =
  | { kind: 'single'; entry: LogEntry }
  | { kind: 'recipe'; recipeId: number; recipeName: string; entries: LogEntry[]; totalCalories: number };

const groups = computed<LogGroup[]>(() => {
  const out: LogGroup[] = [];
  const recipeGroups = new Map<number, LogEntry[]>();

  for (const e of entries.value) {
    if (e.source_recipe_id) {
      if (!recipeGroups.has(e.source_recipe_id)) recipeGroups.set(e.source_recipe_id, []);
      recipeGroups.get(e.source_recipe_id)!.push(e);
    } else {
      out.push({ kind: 'single', entry: e });
    }
  }
  for (const [rid, entries] of recipeGroups) {
    out.push({
      kind: 'recipe',
      recipeId: rid,
      recipeName: entries[0].source_recipe_name ?? 'Recipe',
      entries,
      totalCalories: entries.reduce((s, e) => s + e.calories, 0),
    });
  }
  return out;
});
```

Render:
- **Single entries**: same row as today.
- **Recipe groups**: a parent row showing recipe name + total calories + a single delete button; ingredients listed indented underneath (collapsible). The delete button calls `logApi.deleteRecipeGroup(userId, date, recipeId)` — wipes all three rows in one go.

### D. Home donut charts

Nothing to change. Recipe-sourced entries are normal `log_entries` and are already counted by `/today`.

---

## Phase 6 — Seed data (optional)

Extend `backend/cmd/seed/main.go` to add 1–2 demo recipes (e.g., "Mango Shake" = mango + milk + sugar) and log one for each user. Useful for screenshotting.

---

## Phase 7 — Testing checklist

Manual smoke-test in dev:

- [ ] Create a recipe with 3 ingredients; total calories displayed correctly.
- [ ] Edit recipe name + swap an ingredient → past logs untouched.
- [ ] Search drawer returns mixed foods + recipes; recipes carry the badge.
- [ ] Log a recipe with scale = 1 → 3 entries created, grouped in table.
- [ ] Log same recipe with scale = 0.5 → quantities halved, calories halved.
- [ ] Delete the recipe group → all 3 entries gone in one click.
- [ ] Delete single entry inside a recipe group → only that row removed (regular endpoint).
- [ ] Delete a food that's used in a recipe → 409/400 (ON DELETE RESTRICT).
- [ ] Delete a recipe → recipe + ingredients gone; existing logs intact.
- [ ] Today summary on Home reflects recipe-sourced calories.
- [ ] Charts (W/M/Yr) on UserPage include recipe calories.

---

## Phase 8 — Rollout order

1. Schema + migration (Phase 1).
2. SQL queries + `sqlc generate` (Phase 2).
3. Recipe CRUD handlers and routes (Phase 3a).
4. `LogRecipe` + `DeleteLogEntriesByRecipe` handlers (Phase 3b).
5. Frontend types + API client (Phase 4).
6. Recipe management UI (Phase 5a).
7. Combined picker in AddFoodDrawer (Phase 5b).
8. Log grouping + group-delete (Phase 5c).
9. Seed data (Phase 6).
10. Smoke test (Phase 7).

Each phase is independently testable; backend (1–4) can ship and be exercised via curl before any frontend work begins.

---

## Open considerations (not blockers)

- **Recipe with zero ingredients** is rejected at log time (400). Could also reject at create time — preference call.
- **Editing a recipe while it's actively being logged elsewhere**: not a problem; logs always snapshot.
- **Renaming a recipe**: past logs keep the old `source_recipe_name` snapshot. UI displays the snapshot — correct historical record.
- **Recipe-of-recipes**: explicitly excluded by design. If ever wanted, would require allowing `recipe_ingredients.food_id` to point at recipes too (or a parallel column) plus a cycle check.
