# Research — Foods, Logs, Recipes

Deep dive into how the food domain works today, written as context for adding a per-food **protein** field. Findings are anchored at file paths and line numbers so the follow-up plan can cite them directly.

---

## 1. The three tables that matter

`backend/db/schema.sql` (also embedded as `backend/sql/schema.sql`):

- **`foods`** (lines 9–15) — shared library, one row per ingredient. Columns: `id`, `name`, `unit` (default `'g'`), `calories_per_unit` (REAL, NOT NULL), `created_at`. This is the only place `calories_per_unit` is *canonical*.
- **`log_entries`** (lines 17–29) — per-user, per-day food log. Columns include `food_id` (nullable FK), `date`, and crucially **a snapshot** of the food at log time: `food_name`, `food_unit`, `calories_per_unit`, `quantity`, and a pre-computed `calories = calories_per_unit × quantity`. Two more columns tag entries that came from a recipe: `source_recipe_id` (nullable) and `source_recipe_name` (nullable text snapshot).
- **`recipe_ingredients`** (lines 37–42) — recipe ↔ food many-to-many with a `quantity` (REAL, CHECK > 0) in the food's own unit. `food_id` is `ON DELETE RESTRICT`; `recipe_id` is `ON DELETE CASCADE`. **No nutritional snapshot here** — totals are computed live by joining to `foods`.

`recipes` itself (lines 31–35) carries only `id`, `name`, `created_at`. No `calories_per_unit`, no per-recipe nutrition — it's all derived.

Indexes worth noting: `idx_log_user_date (user_id, date)`, `idx_log_source_recipe (source_recipe_id)`, `idx_recipe_ingredients_recipe (recipe_id)`.

### The dual relationship

`log_entries.food_id` has `ON DELETE SET NULL`. `recipe_ingredients.food_id` has `ON DELETE RESTRICT`. The two FKs encode different intents:

- A food can be deleted from the library *only* if no recipe references it (RESTRICT blocks it — surfaced in `FoodLibrary.vue:87` as `alert("...food may be used by a recipe.")`).
- If a food is deletable (no recipe holds it), its log-entry rows survive with `food_id = NULL` but full name/unit/cpu/calories preserved. This is why the snapshot exists.

---

## 2. Creating a food

### UI entry point — `frontend/src/components/FoodEditor.vue`

This single dialog component handles both create and edit. The discriminator is the `food: Food | null` prop (`FoodEditor.vue:11`): `null` ⇒ create, otherwise edit. `isEdit` is the computed flag (`FoodEditor.vue:17`).

In **create mode** (`food == null`):
- All three inputs are live: `name` (free text), `unit` (a `<select>` over `UNITS = ['g', 'ml', 'oz', 'piece', 'tbsp', 'cup', 'serving']` — `FoodEditor.vue:9`), `calories` (number, min 0, step 0.1).
- There's an optional **AI helper**: clicking "✨ AI" hits `POST /api/ai/calorie-hint` with `{name}` and shows the Gemini text suggestion as advisory copy (`FoodEditor.vue:45–59`). The user still types the number in themselves — the hint never auto-fills. AI helper is hidden in edit mode (template `v-if="isEdit"` split at lines 100–125).
- On save, calls `api.createFood({ name, unit, calories_per_unit })` (`FoodEditor.vue:77–81`).

The library page (`FoodLibrary.vue:67–70`) opens this dialog with `editingFood = null` from the "Add food" button. After save the parent reloads via `loadFoods()` (`FoodLibrary.vue:77–79`).

### API client — `frontend/src/lib/api.ts`

```ts
createFood: (payload: CreateFoodPayload) =>
  request<Food>(`${BASE}/foods`, { method: 'POST', body: JSON.stringify(payload) }),
```
`api.ts:62`. Payload shape in `types.ts:78–82` is exactly `{ name, unit, calories_per_unit }`.

### Backend handler — `backend/handlers/foods.go`

`CreateFood` (`foods.go:31–60`):
1. Decode JSON into `createFoodBody`.
2. Trim `name`; reject empty (400).
3. Trim `unit`; if empty default to `'g'` (the handler, not the schema default, owns this fallback).
4. Reject `calories_per_unit < 0`.
5. Call generated `q.CreateFood(ctx, queries.CreateFoodParams{...})` and return `201 Created` with the inserted row.

SQL is `backend/sql/queries/foods.sql:9–12` — straight `INSERT … RETURNING *`. No transaction needed (single statement).

### Route registration

`backend/main.go:45–48` wires the four food routes. `POST /api/foods` is the create path.

---

## 3. Editing a food

This is the **most subtle flow** in the codebase, and the central reason the snapshot model in `log_entries` exists.

### UI — same `FoodEditor.vue`, edit mode

When opened with a non-null `food`, the dialog shows `name` and `unit` as **read-only display** (template branch starts `FoodEditor.vue:100`). Only `calories` is editable. There's an in-dialog note (`FoodEditor.vue:151–153`):

> Changing this updates all past log entries for {{ name }}.

This is honest UI copy — see backend behaviour below.

On save it calls `api.updateFood(food.id, { calories_per_unit })` (`FoodEditor.vue:74–75`).

`FoodLibrary.vue:72–75` opens this dialog from the pencil button on each food row.

### API contract — narrow `UpdateFoodPayload`

`types.ts:84–86`:
```ts
export interface UpdateFoodPayload {
  calories_per_unit: number
}
```

That's it. The endpoint deliberately accepts no other fields, so name/unit are immutable post-create. This sidesteps:
- Recipe ingredient quantities are stored in the food's unit (`recipe_ingredients.quantity`). A unit change (g→piece) would silently rebase every recipe.
- `log_entries.food_unit` is a snapshot — if you change a food from g to ml, historical entries are wrong without compensation that doesn't really exist.

The plan document explicitly calls this out: `plans/editable-food.md:7–15` (the "Why this is safe" section).

### Backend handler — transactional restamp

`backend/handlers/foods.go:66–119` `UpdateFood`:

1. Parse `id` from path; reject invalid.
2. `GetFood(id)` to distinguish 404 from 500.
3. Decode body; reject negative cpu.
4. **Begin a `BeginTx`** and bind `q := h.Q.WithTx(tx)`.
5. `q.UpdateFoodCalories({ID, CaloriesPerUnit})` — updates `foods.calories_per_unit`.
6. `q.RestampLogEntriesForFood({CaloriesPerUnit, FoodID: &id})` — rewrites every `log_entries` row with this `food_id`. The SQL (`foods.sql:23–27`):
   ```sql
   UPDATE log_entries
   SET calories_per_unit = ?1,
       calories          = ?1 * quantity
   WHERE food_id = ?2;
   ```
7. `tx.Commit()`.

Key invariants this preserves:

- **Atomicity**: the food row and all its dependent log snapshots either all move to the new cpu, or none do. Rollback path is `defer func() { _ = tx.Rollback() }()` (`foods.go:95`).
- **Orphaned logs are untouched**: rows where `food_id IS NULL` (food previously deleted) are excluded by the `WHERE food_id = ?` predicate — by design (see `plans/editable-food.md:14`).
- **Recipes propagate automatically** because recipe totals are live-joined (see §5).
- **Aggregated dashboards self-heal**: `SumCaloriesByDateRange` (`log.sql:40–49`) sums `calories`, which was rewritten in step 6. `GetTodaySummary` does the same (`metrics.sql:20–27`).

### Restamp SQL signature quirk

The generated `RestampLogEntriesForFoodParams.FoodID` is `*int64` (`db/queries/foods.sql.go:107`) because `log_entries.food_id` is nullable. The handler passes `&id`, not `id`. The plan doc flags this explicitly (`plans/editable-food.md:104–105`).

---

## 4. Adding a food to a daily log

There are **two ways** an entry appears in `log_entries`: as a single food add (this section), or as a recipe expansion (§6).

### UI — `frontend/src/components/AddFoodDrawer.vue`

A bottom-sheet overlay opened from `UserPage.vue:328` ("Add food" button) and unmounted on `close`/`added` emits. Local state distinguishes two `Picked` shapes (`AddFoodDrawer.vue:16–33`):

```ts
interface PickedFood   { kind: 'food';   food_id, food_name, food_unit, calories_per_unit, quantity }
interface PickedRecipe { kind: 'recipe'; recipe_id, recipe_name, total_calories, scale }
```

Discovery flow:

- **No query** ⇒ list recent foods. `loadRecent()` (`AddFoodDrawer.vue:58`) calls `api.recentFoods(userId)` which hits `GET /api/users/{id}/recent-foods`. The SQL (`log.sql:7–22`, `GetRecentLoggedFoods`) returns up to 20 of the user's most recently logged single foods (recipe-sourced entries are excluded via `source_recipe_id IS NULL`), one row per distinct `food_name`, with the last `quantity` so the drawer can pre-fill it (`AddFoodDrawer.vue:86–95`).
- **Has query** ⇒ debounced (200 ms) `Promise.all([api.listFoods, api.listRecipes])` — recipes are listed *above* foods in the merged result (`AddFoodDrawer.vue:66–84`). Each result is a `Pickable` discriminated union (`types.ts:128–131`).

Confirming a single food:

```ts
const payload: AddLogPayload = {
  food_id, food_name, food_unit, calories_per_unit,
  quantity: Number(picked.quantity),
  date: props.date,
}
await api.addLog(userId, payload)
```
`AddFoodDrawer.vue:132–142`. Note that the client sends both `food_id` *and* a full snapshot — the backend trusts the client's snapshot rather than re-reading from `foods`.

After success the drawer emits `added`, which `UserPage.vue:200–203` handles by closing the drawer and calling `loadLog()` to re-read entries for the day.

### API client

```ts
addLog: (userId, payload) =>
  request<LogEntry>(`${BASE}/users/${userId}/log`, { method: 'POST', body: ... })
```
`api.ts:74–78`.

### Backend handler

`backend/handlers/log.go:36–95` `AddLogEntry`:
- Decodes `addLogBody` (`food_id *int64`, `food_name`, `food_unit`, `calories_per_unit`, `quantity`, `date`).
- Validates: name/unit trimmed-non-empty, `quantity > 0`, `cpu >= 0`, `date` matches `^\d{4}-\d{2}-\d{2}$` (`handlers.go:55–59`).
- Inserts via `q.AddLogEntry(...)` with `Calories: body.CaloriesPerUnit * body.Quantity` (`log.go:86`) — calories are computed server-side, not from the client.
- `SourceRecipeID` and `SourceRecipeName` are forced to `nil` here (`log.go:87–88`) — this is what marks the entry as a single-food (non-recipe) log.

SQL is `backend/sql/queries/log.sql:24–29` — a 10-column `INSERT … RETURNING *`.

### What the snapshot guarantees

At write time, the row carries `food_name`, `food_unit`, `calories_per_unit` *as the client sent them*. So:

- Deleting a food later doesn't break old log rows (FK is `SET NULL`).
- Editing a food's cpu **does** rewrite old rows for entries that still hold the FK (§3). Editing name/unit is not possible by design.
- Renaming a food in the AddFoodDrawer UI is not possible either — the snapshot mirrors the library row at the moment of add.

---

## 5. Recipes — composition, not snapshot

Recipes are stored as *recipe + many recipe_ingredients*; no nutritional totals are persisted on either table. Totals are derived at query time.

### Read paths that compute totals

- `ListRecipes` (`recipes.sql:14–24`):
  ```sql
  SELECT r.id, r.name, r.created_at,
         CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories
  FROM recipes r
  LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
  LEFT JOIN foods f               ON f.id          = ri.food_id
  GROUP BY r.id
  ```
- `GetRecipeIngredients` (`recipes.sql:27–38`) returns one row per ingredient with `food_name`, `food_unit`, `calories_per_unit` joined from `foods`. The handler (`handlers/recipes.go:85–88`) then sums `cpu × qty` to get the recipe's `total_calories` for `recipeDetailResponse`.

**Implication**: a food's `calories_per_unit` change immediately re-rates every recipe that uses it, no migration needed. This is the inverse of `log_entries`: recipes are live-joined, logs are snapshotted.

### Create / update a recipe

`POST /api/recipes` (`handlers/recipes.go:98–138`) and `PUT /api/recipes/{id}` (`recipes.go:140–199`) both wrap a transaction:

- Create: `CreateRecipe(name)` then loop `AddRecipeIngredient`.
- Update: name update + `ClearRecipeIngredients(id)` + loop `AddRecipeIngredient`. The clear-and-re-insert pattern means ingredient row IDs are not stable across updates.

Validation (`recipes.go:30–47`): name non-empty, at least one ingredient, each ingredient has `food_id > 0` and `quantity > 0`.

UI counterpart: `RecipeEditor.vue` — search-and-add foods to a draft list, edit per-ingredient quantity, live-compute "Total per serving" client-side (`RecipeEditor.vue:40–49`) using the joined `calories_per_unit` returned by `api.listFoods` and the ingredient list returned by `api.getRecipe`.

Dedupe: ingredients are de-duplicated by `food_id` on the client (`RecipeEditor.vue:106`), so adding the same food twice is a no-op.

### Logging a recipe → expansion into N log entries

`AddFoodDrawer.vue:151–170` handles the "logged a recipe" branch: it calls `api.logRecipe({ recipe_id, scale, date })` where `scale` defaults to 1 (think "servings": 0.5 = half-batch, 2 = double-batch).

`POST /api/users/{id}/log/recipe` (`handlers/recipes.go:220–299`):

1. Validates `recipe_id > 0`, `scale > 0`, `date` format.
2. Loads recipe + ingredients.
3. **Opens a tx** and inserts one `log_entries` row per ingredient:
   ```go
   qty := ing.Quantity * body.Scale
   AddLogEntry{
     UserID, FoodID: &foodID, Date: body.Date,
     FoodName: ing.FoodName, FoodUnit: ing.FoodUnit,
     CaloriesPerUnit: ing.CaloriesPerUnit,
     Quantity: qty,
     Calories: ing.CaloriesPerUnit * qty,
     SourceRecipeID: &recipeID,
     SourceRecipeName: &recipeName,
   }
   ```
4. Commits all at once — failure mid-loop rolls everything back.

**The snapshot here is taken at the moment of logging**, just like single-food adds. Each ingredient becomes its own row with `food_name`/`food_unit`/`cpu` copied from the live `foods` table *via the join* (`GetRecipeIngredients`). The `source_recipe_id`/`source_recipe_name` columns carry forward the recipe's identity.

### Recipe deletion vs ingredient deletion

- Deleting a recipe (`DELETE /api/recipes/{id}`, `recipes.go:201–212`) cascades to `recipe_ingredients` (FK `ON DELETE CASCADE`). Past log entries are *unaffected* — they just keep their `source_recipe_id` pointing at a now-nonexistent row (no FK from `log_entries.source_recipe_id` to `recipes`, intentionally — schema shows no `REFERENCES` on this column, line 27).
- Deleting a food used by a recipe is blocked at the DB layer (`recipe_ingredients.food_id ON DELETE RESTRICT`); the error bubbles up to the UI as an alert (`FoodLibrary.vue:81–89`).

### Recipe group rendering on the log page

`UserPage.vue:65–98` groups `log_entries` by `source_recipe_id`. Rows with the same recipe id are collapsed into a `LogGroup` (`kind: 'recipe'`) with a header row, a "Recipe" badge, ingredient count, summed calories, and a single trash button. Single rows (no source recipe) render as `{ kind: 'single' }`.

Deletion is symmetrical:
- Single row → `DELETE /api/users/{id}/log/{eid}` (`log.go:97–116`), enforces ownership via `WHERE id = ? AND user_id = ?`.
- Recipe group → `DELETE /api/users/{id}/log/recipe?date=...&source_recipe_id=...` (`log.go:118–143`), which uses `DeleteLogEntriesByRecipe` (`log.sql:34–38`) to atomically wipe all rows from that recipe on that day.

---

## 6. Read paths that aggregate across logs

A new per-food nutrient (e.g. protein) needs to flow through every place calories already does. The aggregating SQL today:

| Where | File:line | What it sums |
|---|---|---|
| Day total (UI footer) | `UserPage.vue:60–62` | client-side `reduce` over `entries.calories` |
| Today summary (Home donut) | `metrics.sql:20–27` `GetTodaySummary` | server-side `SUM(le.calories)` for `date('now')` |
| Charts range | `log.sql:40–49` `SumCaloriesByDateRange` | per-day `SUM(calories)` over a date range |
| Recipe total (list) | `recipes.sql:18` | live `SUM(f.calories_per_unit * ri.quantity)` |
| Recipe total (detail) | `handlers/recipes.go:85–88` | client-summed in Go from joined rows |
| Recipe total (editor) | `RecipeEditor.vue:40–49` | client-summed in TS, live as you edit |
| Preview when adding | `AddFoodDrawer.vue:50–56` | client preview `n × cpu` (food) or `n × total_calories` (recipe) |

Notice the pattern split:
- **`log_entries` rows already carry `calories`** — aggregation is a pure SUM, fast and trivially correct (provided the snapshot is correct).
- **Recipes carry no pre-computed total** — aggregation is a join, slightly heavier but self-healing on food edits.

---

## 7. Generated code & migration plumbing

- sqlc config: `backend/sqlc.yaml`. Hand-written SQL lives in `backend/sql/queries/*.sql`; generated Go lands in `backend/db/queries/`. Regenerate with `cd backend && sqlc generate`. Generated files are committed.
- The embedded schema is `backend/db/schema.sql` (note: separate from `backend/sql/schema.sql` — the `db/` copy is what's `//go:embed`-ed at `db/db.go:14–15`). Both should stay in sync; the user-config plan calls this out (`plans/user-config-plan.md:47`).
- `db.Init` (`backend/db/db.go:22–48`):
  - opens SQLite, enables `PRAGMA foreign_keys = ON`,
  - runs three `ensureColumn` calls for legacy DBs (covers `source_recipe_id`, `source_recipe_name`, `target_calories`),
  - then `dropColumnIfExists("daily_metrics", "target_calories")`,
  - then runs the embedded schema with `CREATE TABLE IF NOT EXISTS` semantics statement-by-statement.
- `ensureColumn` (`db.go:62–96`) — checks `sqlite_master` for the table, then `PRAGMA table_info` for the column, then `ALTER TABLE … ADD COLUMN …` if missing. Idempotent. Used to add nullable columns to existing rows safely.
- `dropColumnIfExists` (`db.go:98–136`) — same shape, runs `ALTER TABLE … DROP COLUMN` if present. Requires SQLite ≥ 3.35.

These two helpers are the template for any future schema change (including a `protein_per_unit` addition).

---

## 8. Snapshot vs live-join — the central design choice

The single sentence that captures the whole food domain:

> **Recipes read foods live; log entries snapshot foods at write time.**

This is why:

- Recipe totals "magically" pick up food edits — they're a join.
- Log entries don't, and so the edit path has to *explicitly* restamp them (`UpdateFood` + `RestampLogEntriesForFood`).
- A food's nutritional fields therefore live in **three places** logically:
  1. `foods` (canonical).
  2. `recipe_ingredients` — *not stored*, joined.
  3. `log_entries` — *stored as a snapshot*, kept in sync by the restamp on edit, severed (`food_id = NULL`) on delete.

Any new nutritional column has to walk all three.

---

## 9. Implications for adding `protein` (per food)

Pulling the above together — what a `protein_per_unit` column would have to touch, mechanically:

### Schema
- `foods.protein_per_unit REAL NOT NULL DEFAULT 0` (default avoids breaking insert paths that don't yet send it, and makes `ALTER TABLE … ADD COLUMN` well-defined on legacy DBs — same trick `target_calories` used in `plans/user-config-plan.md`).
- `log_entries.protein_per_unit REAL NOT NULL DEFAULT 0` and `log_entries.protein REAL NOT NULL DEFAULT 0` — the snapshot + pre-computed pair, mirroring `calories_per_unit` / `calories`.
- `recipe_ingredients` — **no change**. Protein totals come from the live join, just like calories.
- Run two `ensureColumn` calls in `db.Init` for legacy DBs.

### sqlc queries
- `foods.sql`: extend `CreateFood`, `UpdateFoodCalories` (or rename to `UpdateFoodNutrition`), and **`RestampLogEntriesForFood`** to also write `protein_per_unit` and `protein = ? * quantity`.
- `log.sql`: extend `AddLogEntry` to accept and insert protein; `GetLogForDate` and `GetRecentLoggedFoods` are `SELECT *` style and just need the regenerated structs.
- `log.sql`: add (or extend) the aggregate — `SumCaloriesByDateRange` becomes `SumNutritionByDateRange` returning `(date, total_calories, total_protein)`. Same shape, one extra column.
- `metrics.sql`: extend `GetTodaySummary` to also return `SUM(protein)` for today.
- `recipes.sql`: extend `ListRecipes` to also `SUM(f.protein_per_unit * ri.quantity)`. `GetRecipeIngredients` already returns the food's columns via join — just include `protein_per_unit` and the Go-side aggregator (`handlers/recipes.go:85–88`) sums it.

### Handlers
- `CreateFood` (`foods.go:31–60`) — add validation `protein_per_unit >= 0`.
- `UpdateFood` (`foods.go:66–119`) — accept `protein_per_unit` in the body; both `UpdateFoodCalories` and `RestampLogEntriesForFood` now write protein too. The transactional guarantee already in place still holds.
- `AddLogEntry` (`log.go:36–95`) — pass `protein_per_unit` and `protein = protein_per_unit * quantity` through. Validate `>= 0`.
- `LogRecipe` (`handlers/recipes.go:220–299`) — when expanding an ingredient, the same `protein_per_unit` flows from `GetRecipeIngredients` (post-join) and the loop multiplies by `qty` to fill `protein`.
- `GetRecipe`, `ListRecipes` — total computation extends to total_protein.
- `GetTodaySummary` — return protein alongside calories.
- `GetMetrics` — already merges per-day aggregates from `SumCaloriesByDateRange`; extend the merge.

### TypeScript types & API client
- `types.ts`: add `protein_per_unit` to `Food`, `LogEntry`, `RecentFood`; add `protein` to `LogEntry`; extend `RecipeListItem` / `RecipeWithIngredients` with `total_protein`. Extend `AddLogPayload` and `CreateFoodPayload`/`UpdateFoodPayload`.
- `api.ts`: payload shapes are JSON-pass-through so the file changes are minimal beyond the type imports.

### UI
- `FoodEditor.vue`: add a protein input in both create and edit modes (calories is already the only edit-mode-editable field — protein should join it). Update the "Changing this updates all past log entries" note to be true for both.
- `FoodLibrary.vue` row: surface `g protein / 1 {unit}` next to calories, similar to line 154–156.
- `AddFoodDrawer.vue`: live preview already shows `≈ N kcal`; add `· N g protein`. Mirror in both the food branch and the recipe branch.
- `UserPage.vue` log table: add a protein column (or fold into existing total row at lines 322–325).
- `Home.vue` summaries: optionally add a protein number next to the kcal donut.
- `ProgressCharts.vue`: protein could be a new line/bar later — separable from the initial cut.

### Test surface (informal, no test framework currently in repo)
- Manual: same checklist as `plans/editable-food.md:399–411`, repeated for protein.
- Particularly: editing a food's protein restamps every log entry where `food_id` matches, and recipes show the new total without further action.

---

## 10. Gotchas to remember

1. **Two `schema.sql` files** — `backend/db/schema.sql` is the embedded one; `backend/sql/schema.sql` is referenced by sqlc tooling. Keep both in sync.
2. **`food_id` is nullable in `log_entries`** — the restamp predicate `WHERE food_id = ?` deliberately skips orphaned rows. Don't broaden to a name match; deleted-and-recreated foods are semantically different objects.
3. **Recipe `total_calories` for a deleted ingredient food**: `recipe_ingredients.food_id` is `ON DELETE RESTRICT`, so this state can't occur. But the `LEFT JOIN` in `ListRecipes` (`recipes.sql:19–21`) means even if it ever does, missing food rows contribute 0 instead of dropping the recipe — defensive, but won't trip in practice.
4. **`SourceRecipeID` has no FK** (schema line 27). Deleting a recipe leaves dangling `source_recipe_id` values in `log_entries` — by design, so historical logs survive recipe deletion.
5. **`RestampLogEntriesForFoodParams.FoodID *int64`** — sqlc makes it a pointer because the column is nullable. Pass `&id`, not `id`.
6. **Schema migration helpers run before `CREATE TABLE IF NOT EXISTS`** (`db.go:30–46`). On a fresh DB the table doesn't exist yet so `ensureColumn` no-ops; the schema then creates the table with the new column inline. On legacy DBs the migration adds the column first so subsequent `CREATE INDEX` statements have the column to reference.
7. **`AddLogEntry` validates server-side but trusts the client snapshot.** The handler does not re-read `foods` to verify the snapshot matches — it just stores what the client sent (`log.go:78–94`). This is fine in practice because the snapshot is intentionally a point-in-time copy, not a derived view.
8. **`GetRecentLoggedFoods` filters out recipe-sourced rows** (`log.sql:18` — `inner_le.source_recipe_id IS NULL`). The drawer's "recent" list is a single-food shortcut; recipes are surfaced through the search results path instead. A protein column will need the same treatment if it's surfaced in this list.

---

## 11. File index (quick reference)

Backend
- `backend/db/schema.sql` — embedded schema
- `backend/sql/schema.sql` — sqlc-side schema (mirror)
- `backend/sql/queries/foods.sql` — `ListFoods`, `GetFood`, `CreateFood`, `DeleteFood`, `UpdateFoodCalories`, `RestampLogEntriesForFood`
- `backend/sql/queries/log.sql` — log CRUD + `GetRecentLoggedFoods` + `SumCaloriesByDateRange`
- `backend/sql/queries/recipes.sql` — recipe CRUD + `GetRecipeIngredients` + `ListRecipes` (with total_calories)
- `backend/sql/queries/metrics.sql` — `UpsertMetrics`, `GetMetricsRange`, `GetTodaySummary`
- `backend/db/queries/*.sql.go` — generated; do not hand-edit
- `backend/db/db.go` — `Init`, `ensureColumn`, `dropColumnIfExists`
- `backend/handlers/foods.go` — Create/Update (transactional restamp)/Delete/List
- `backend/handlers/log.go` — Add, delete-single, delete-by-recipe, recent
- `backend/handlers/recipes.go` — CRUD + `LogRecipe` expansion (transactional)
- `backend/handlers/metrics.go` — range + today summary
- `backend/main.go` — route table
- `backend/cmd/seed/main.go` — dev seed (users, foods, 14 days of entries, two demo recipes)

Frontend
- `frontend/src/lib/types.ts` — all TS interfaces
- `frontend/src/lib/api.ts` — typed fetch client
- `frontend/src/router/index.ts` — three routes
- `frontend/src/stores/user.ts` — Pinia user store
- `frontend/src/views/Home.vue` — user list, donut per user, add-user dialog
- `frontend/src/views/UserPage.vue` — log table (with recipe grouping), metrics inputs, progress charts
- `frontend/src/views/FoodLibrary.vue` — tabbed Foods + Recipes
- `frontend/src/components/FoodEditor.vue` — create + edit dialog (only cpu editable in edit mode)
- `frontend/src/components/RecipeEditor.vue` — create + edit dialog (clear-and-reinsert ingredients on save)
- `frontend/src/components/AddFoodDrawer.vue` — picker for foods+recipes, recent shortcut, scale input for recipes
- `frontend/src/components/DonutChart.vue` / `ProgressCharts.vue` — visualisation only

Plans (for reference, not authority)
- `plans/editable-food.md` — design doc for the edit-food flow; explains restamp rationale and the immutable-name/unit choice.
- `plans/user-config-plan.md` — design doc for moving `target_calories` to users; useful template for nullable→non-null migrations with `ALTER TABLE ADD COLUMN ... NOT NULL DEFAULT`.
- `plans/recipe-plan.md` — original recipe design.
