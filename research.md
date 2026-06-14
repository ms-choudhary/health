# Research: Foods, Recipes, and the Log

A deep-dive into how **foods** and **recipes** are modeled, how they relate, and
how each is added to a user's daily **log** in the Health calorie tracker.

Stack: Vue 3 + shadcn-vue frontend, Go (`net/http`) + SQLite + sqlc backend.

- Backend handlers: `backend/handlers/`
- SQL source of truth: `backend/sql/queries/*.sql` (+ `backend/sql/schema.sql`)
- Generated DB layer: `backend/db/queries/*.sql.go` (sqlc, **do not edit**)
- Schema applied at runtime: `backend/db/schema.sql`
- Frontend types/api: `frontend/src/lib/{types.ts,api.ts}`
- Frontend UI: `frontend/src/components/` and `frontend/src/views/`

---

## 1. The data model

### 1.1 `foods` — the reusable nutrition catalog

```sql
CREATE TABLE foods (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  name              TEXT NOT NULL,
  unit              TEXT NOT NULL DEFAULT 'g',
  calories_per_unit REAL NOT NULL,
  protein_per_unit  REAL NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL DEFAULT (date('now'))
);
```

A food is a **per-unit nutrition template**: "X kcal and Y g protein per 1 `unit`".
The unit is a free-text label chosen from a frontend whitelist
(`g, ml, oz, piece, tbsp, cup, serving` — see `FoodEditor.vue` `UNITS`), but the
backend accepts any non-empty string and defaults to `g`.

Key properties:
- Foods are **global / shared**, not owned by a user. There is no `user_id`.
- Nutrition is stored *per unit*, so the actual calories of a log entry are
  always `calories_per_unit * quantity`, computed at log time.
- `name` is not unique — duplicates are allowed.

### 1.2 `recipes` + `recipe_ingredients` — compositions of foods

```sql
CREATE TABLE recipes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE recipe_ingredients (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  food_id   INTEGER NOT NULL REFERENCES foods(id)   ON DELETE RESTRICT,
  quantity  REAL    NOT NULL CHECK(quantity > 0)
);
```

A recipe is just a **name plus a list of (food, quantity) pairs**. It stores **no
nutrition of its own** — a recipe's calories/protein are always *derived* by
summing its ingredients' per-unit values times their quantities. The quantity on
each ingredient is expressed in that food's own unit.

Critically, **a recipe represents exactly one serving.** There is no servings
field on the recipe; `ListRecipes`/`GetRecipe` totals are "per serving," and
scaling to multiple servings happens only at log time (see §4.2).

Referential integrity is the relationship's backbone:
- `recipe_ingredients.recipe_id → recipes.id ON DELETE CASCADE`: deleting a recipe
  drops its ingredient rows automatically.
- `recipe_ingredients.food_id → foods.id ON DELETE RESTRICT`: **you cannot delete a
  food that is referenced by any recipe.** The DB rejects it, the handler returns a
  500, and `FoodLibrary.vue`'s `deleteFood` surfaces a generic
  "Cannot delete — food may be used by a recipe." alert.

### 1.3 `log_entries` — the denormalized daily log (the heart of it)

```sql
CREATE TABLE log_entries (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  food_id             INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  date                TEXT NOT NULL,                 -- 'YYYY-MM-DD'
  food_name           TEXT NOT NULL,
  food_unit           TEXT NOT NULL,
  calories_per_unit   REAL NOT NULL,
  protein_per_unit    REAL NOT NULL DEFAULT 0,
  quantity            REAL NOT NULL,
  calories            REAL NOT NULL,                 -- = calories_per_unit * quantity
  protein             REAL NOT NULL DEFAULT 0,       -- = protein_per_unit  * quantity
  source_recipe_id    INTEGER,                       -- nullable, NOT a FK
  source_recipe_name  TEXT,
  source_recipe_servings REAL
);
```

This is the most important table to understand. **Every logged item — whether a
plain food or a recipe — is stored as one or more rows in `log_entries`, and each
row is fully self-contained (denormalized).**

The entry copies a *snapshot* of the food's name, unit, and per-unit nutrition,
**and** the precomputed `calories`/`protein`. It does not rely on joining back to
`foods` to display or total a day's log. This snapshotting has deliberate
consequences:

- A log entry survives the source food's deletion: `food_id` is
  `ON DELETE SET NULL`, but `food_name`, `food_unit`, and the nutrition columns
  remain, so history stays intact and readable.
- Editing a food does **not** automatically change past entries through joins —
  the snapshot is frozen. (There is an explicit *re-stamp* step for this; see §6.)

Two ways an entry relates to a food/recipe:
- **Plain food entry:** `food_id` set, `source_recipe_id` / `source_recipe_name` /
  `source_recipe_servings` all `NULL`.
- **Recipe-derived entry:** one row **per ingredient**, each with `food_id` set to
  the ingredient's food **and** `source_recipe_id` / `source_recipe_name` /
  `source_recipe_servings` populated so the rows can be regrouped as a recipe.

Note `source_recipe_id` is intentionally **not a foreign key** — it's a soft
reference. Deleting a recipe does not touch already-logged entries (confirmed by
the `FoodLibrary.vue` "Past logs are unaffected." confirmation text).

Indexes: `idx_log_user_date (user_id, date)` powers the per-day log fetch;
`idx_log_source_recipe (source_recipe_id)` supports recipe grouping/recent queries.

### 1.4 Relationship summary

```
users ──1:N──> log_entries <── soft ref ── recipes ──1:N──> recipe_ingredients ──N:1──> foods
                   │                                                                       ▲
                   └──────────────── food_id (ON DELETE SET NULL) ────────────────────────┘

recipe_ingredients.food_id ── ON DELETE RESTRICT ──> foods   (blocks food deletion)
recipe_ingredients.recipe_id ── ON DELETE CASCADE ──> recipes
log_entries.user_id ── ON DELETE CASCADE ──> users
```

- A **food** can appear in many recipes and many log entries.
- A **recipe** is an ordered bag of foods; it owns no nutrition.
- A **log entry** is a frozen snapshot; recipe membership is reconstructed from the
  `source_recipe_*` columns, not a live join.

---

## 2. Foods: lifecycle (CRUD)

API surface (`main.go`, `handlers/foods.go`):

| Method | Route             | Handler      | Notes |
|--------|-------------------|--------------|-------|
| GET    | `/api/foods?q=`   | `ListFoods`  | substring search on name, `ORDER BY name` |
| POST   | `/api/foods`      | `CreateFood` | |
| PUT    | `/api/foods/{id}` | `UpdateFood` | **nutrition only**, re-stamps log (§6) |
| DELETE | `/api/foods/{id}` | `DeleteFood` | blocked by `RESTRICT` if used in a recipe |

- **Search** (`ListFoods`): `WHERE name LIKE '%' || ? || '%'`. Empty `q` matches all.
- **Create**: trims name (required), defaults blank unit to `g`, rejects negative
  calories/protein. Returns the created row (201).
- **Update**: only `calories_per_unit` and `protein_per_unit` are editable —
  **name and unit are immutable after creation**. The frontend `FoodEditor.vue`
  reflects this: in edit mode it shows name/unit as read-only and only exposes the
  two numeric fields. Update runs in a transaction together with the log re-stamp.
- **Delete**: hard delete; fails (500 → user-facing alert) when a recipe references
  the food.

### AI calorie hint (food creation aid)
`POST /api/ai/calorie-hint` (`handlers/ai.go`) takes a food `name`, calls Google
Gemini (`gemini-2.5-flash`) with "ANSWER IN ONE LINE: how much calories and protein
(g) in {name}?", and returns a one-line `hint` string. It's purely advisory — the
user still types the numbers in `FoodEditor.vue`. Disabled (503) when
`GEMINI_API_KEY` is unset.

---

## 3. Recipes: lifecycle (CRUD)

API surface (`main.go`, `handlers/recipes.go`):

| Method | Route                | Handler        | Notes |
|--------|----------------------|----------------|-------|
| GET    | `/api/recipes?q=`    | `ListRecipes`  | each row carries derived totals |
| GET    | `/api/recipes/{id}`  | `GetRecipe`    | recipe + ingredients + totals |
| POST   | `/api/recipes`       | `CreateRecipe` | transactional |
| PUT    | `/api/recipes/{id}`  | `UpdateRecipe` | replace-all ingredients, transactional |
| DELETE | `/api/recipes/{id}`  | `DeleteRecipe` | cascades ingredient rows |

### 3.1 Validation (`validateRecipeBody`)
Shared by create and update:
- `name` required (trimmed).
- **At least one ingredient required.**
- Every ingredient: `food_id > 0` and `quantity > 0`.

### 3.2 Derived totals
Both `ListRecipes` (SQL aggregate) and `GetRecipe` (Go loop) compute:
```
total_calories = Σ ingredient.calories_per_unit * ingredient.quantity
total_protein  = Σ ingredient.protein_per_unit  * ingredient.quantity
```
`LEFT JOIN` + `COALESCE(..., 0)` mean a recipe with no joinable ingredients totals
to 0 rather than disappearing. These totals are **per serving**.

### 3.3 Create
In a transaction: insert the `recipes` row, then loop `AddRecipeIngredient` for each
ingredient. Any failure rolls back the whole recipe. Returns the bare `Recipe`
(no ingredients/totals in the response).

### 3.4 Update = "replace ingredients wholesale"
In a transaction (`UpdateRecipe`):
1. `UpdateRecipeName`
2. `ClearRecipeIngredients` (delete all rows for the recipe)
3. re-insert every ingredient from the payload

There is no per-ingredient diffing — the editor always sends the full ingredient
list and the backend rebuilds it. This regenerates `recipe_ingredients.id`s.

> **Note:** `UpdateRecipe` responds with `queries.Recipe{ID: id, Name: name}` —
> `created_at` is empty in that response (the row isn't re-read).

### 3.5 Delete
`DeleteRecipe` is a hard delete; `ON DELETE CASCADE` removes ingredient rows.
**Already-logged entries are untouched** because `source_recipe_id` is not a FK.
Frontend confirms with "Past logs are unaffected."

---

## 4. Adding things to the log

There are **two distinct logging paths**, and they are the core of the
food/recipe distinction at runtime.

### 4.1 Logging a plain food — `POST /api/users/{id}/log`

`handlers/log.go` `AddLogEntry`, body `{ food_id, quantity, date }`:
1. Validate: `food_id > 0`, `quantity > 0`, `date` matches `^\d{4}-\d{2}-\d{2}$`.
2. `GetFood(food_id)` (404 if missing) — fetches **current** nutrition.
3. Insert **one** `log_entries` row, snapshotting `food_name`, `food_unit`,
   `calories_per_unit`, `protein_per_unit`, and computing
   `calories = calories_per_unit * quantity`, `protein = protein_per_unit * quantity`.
   All `source_recipe_*` columns are `NULL`.

One food log = one row.

### 4.2 Logging a recipe — `POST /api/users/{id}/log/recipe`

`handlers/recipes.go` `LogRecipe`, body `{ recipe_id, servings, date }`:
1. Validate: `recipe_id > 0`, `servings > 0`, valid `date`.
2. `GetRecipe` (404 if missing) and `GetRecipeIngredients`
   (400 "recipe has no ingredients" if empty).
3. In a transaction, **insert one `log_entries` row per ingredient**, each with:
   - `food_id`, `food_name`, `food_unit`, and per-unit nutrition snapshotted from
     the ingredient (which itself joins live `foods` data at log time);
   - `quantity = ingredient.quantity * servings` (this is where servings scaling
     happens);
   - `calories = calories_per_unit * quantity`, `protein` likewise;
   - `source_recipe_id = recipe.id`, `source_recipe_name = recipe.name`,
     `source_recipe_servings = servings` — **the same trio repeated on every row.**

So logging a 3-ingredient recipe at 2 servings creates **3 rows**, each scaled ×2,
all tagged with the same `source_recipe_id` and `source_recipe_servings = 2`.

> **Important consequence:** a recipe is "exploded" into ingredient rows at log
> time. The log does **not** keep a reference to a single recipe line item — it
> keeps the constituent foods, tagged so the UI can regroup them. This means later
> edits to the recipe definition do not retroactively change what was logged, and
> the recipe's name/servings are themselves snapshotted onto each row.

### 4.3 Why denormalize per ingredient?
- Per-ingredient rows make `food_id`-based features work uniformly (the per-food
  re-stamp in §6 updates recipe-logged rows too, since they carry `food_id`).
- Daily totals are a simple `SUM(calories)` over all rows regardless of source.
- The `source_recipe_*` tags are pure presentation metadata for grouping/labeling.

---

## 5. Reading & displaying the log

### 5.1 Fetch a day — `GET /api/users/{id}/log?date=`
`GetLogForDate`: `WHERE user_id=? AND date=? ORDER BY id`. Returns a flat list of
all entries (food rows and recipe-ingredient rows intermixed, in insertion order).

### 5.2 Regrouping in the UI (`UserPage.vue` `groups` computed)
The frontend reconstructs recipe groupings from the flat list:
- Entries with `source_recipe_id == null` become `{ kind: 'single', entry }`.
- Entries sharing a `source_recipe_id` are collected into one
  `{ kind: 'recipe', recipeId, recipeName, servings, entries, totalCalories,
  totalProtein }` group, preserving first-seen order.
- A recipe group renders as a header row (name + "Recipe" badge + servings +
  summed cal/protein) with its ingredient rows indented beneath ("↳ ...").

Daily totals (`totalCalories`, `totalProtein`) simply `reduce` over the **flat**
`entries` array, so grouping is purely visual and never affects the sum.

### 5.3 Today summary — `GET /api/users/{id}/today`
`GetTodaySummary` sums `log_entries.calories`/`.protein` for `date('now')` and
returns it alongside the user's targets. Used for dashboard progress.

---

## 6. Editing food nutrition re-stamps the log (the subtle bit)

When a food's nutrition changes (`UpdateFood`), past log entries would otherwise be
stale snapshots. The handler runs, **in one transaction**:
1. `UpdateFoodNutrition` — update the `foods` row.
2. `RestampLogEntriesForFood`:
   ```sql
   UPDATE log_entries
   SET calories_per_unit = ?1, calories = ?1 * quantity,
       protein_per_unit  = ?2, protein  = ?2 * quantity
   WHERE food_id = ?3;
   ```

Implications:
- **All historical entries** for that food are recalculated to the new per-unit
  values (quantity preserved). The UI warns: "Changing these updates all past log
  entries for {name}."
- This applies across **all users** (entries aren't filtered by user) and across
  **all dates**.
- **Recipe-derived entries are also re-stamped** because they carry `food_id`. So
  changing a food retroactively changes both plain-food rows and recipe-ingredient
  rows already in the log — but the recipe's *own* derived totals (computed live in
  §3.2) also reflect the new value, keeping them consistent.
- Entries whose `food_id` was nulled (food previously deleted) are not matched and
  keep their old snapshot.

Only nutrition is editable; name/unit changes aren't possible, so the snapshotted
`food_name`/`food_unit` never drift.

---

## 7. Recent items (smart log-entry shortcuts)

`GET /api/users/{id}/recent-foods` (`handlers/log.go` `GetRecentFoods`) powers the
"Recent" list in the add-to-log drawer. It merges two queries over a **7-day
window** (`recentWindowDays`, floor = now − 7 days) and caps the merged list at
**20** items (`recentItemsCap`):

- `GetRecentLoggedFoods`: most recent **plain-food** entry per `food_id`
  (`source_recipe_id IS NULL AND food_id IS NOT NULL`), carrying `last_quantity`
  (the quantity used last time) and `max_id` for ordering. Limit 50.
- `GetRecentLoggedRecipes`: most recent entry per `source_recipe_id`
  (`source_recipe_id IS NOT NULL`), joined back to `recipes` + `recipe_ingredients`
  + `foods` to recompute **per-serving** totals live, plus `last_servings`
  (`COALESCE(source_recipe_servings, 1)`). Limit 20.

Both are merged into a `recentItem` union, sorted by `maxID` desc (most recently
logged first), truncated to 20. The `kind` field (`"food"` | `"recipe"`)
discriminates them on the frontend (`RecentItem` union in `types.ts`).

Selecting a recent **food** pre-fills its last quantity; selecting a recent
**recipe** pre-fills its last servings — both editable before confirming.

---

## 8. The add-to-log drawer (`AddFoodDrawer.vue`) — unified UX

A single bottom-sheet drawer handles both logging paths:

- **Empty query** → shows the merged **Recent** list (§7).
- **Typed query** → debounced (200 ms) parallel search of `listFoods(q)` **and**
  `listRecipes(q)`; results are merged into a `Pickable` union (recipes listed
  first, then foods) under a "Library" heading.
- Picking an item sets `picked` to either a `PickedFood` (quantity input, unit
  label, live `≈ kcal · g` preview using `calories_per_unit`/`protein_per_unit`) or
  a `PickedRecipe` (servings input, "per serving" totals, preview using
  `total_calories`/`total_protein`).
- **Confirm** branches by `kind`:
  - food → `api.addLog(userId, { food_id, quantity, date })`
  - recipe → `api.logRecipe(userId, { recipe_id, servings, date })`
- On success it emits `added`; `UserPage.vue` closes the drawer and reloads the log.

The drawer also tracks `window.visualViewport` to stay pinned above the iOS
on-screen keyboard (a layout-viewport quirk handled in `onViewportChange`).

---

## 9. Deleting log entries

- **Single entry** — `DELETE /api/users/{id}/log/{eid}` (`DeleteLogEntry`):
  `DELETE ... WHERE id=? AND user_id=?` (the `user_id` guard scopes it to the owner).
- **Whole recipe group** — `DELETE /api/users/{id}/log/recipe?date=&source_recipe_id=`
  (`DeleteLogEntriesByRecipe`): deletes all rows matching `user_id + date +
  source_recipe_id`, i.e. removes every ingredient row of that recipe **for that
  day** in one shot. This is the inverse of the per-ingredient explosion at log
  time, scoped by date so it doesn't wipe the same recipe on other days.

`UserPage.vue` wires the trash icon on a recipe header to `removeRecipeGroup` and
on a single/ingredient row to `removeEntry`, with optimistic local filtering of
`entries` after the API call.

---

## 10. Cross-cutting specifics & gotchas

- **Per-unit, computed-at-write nutrition.** `log_entries.calories`/`.protein` are
  materialized at insert time and only change via the food re-stamp. Recipe totals
  are always derived on read.
- **Snapshot vs. live.** Log rows are snapshots (denormalized); recipe totals (in
  list/detail/recent) are computed live from current food nutrition. After a food
  edit, the re-stamp keeps already-logged rows in sync with the live recipe totals.
- **Foods are immutable in name/unit; recipes are fully replaceable.** Editing a
  recipe rebuilds its ingredient set wholesale (new ingredient row ids).
- **`food_id` nullability propagates types.** `*int64` in Go, `number | null` in
  TS; recipe-tag columns are likewise nullable pointers/`| null`.
- **Recipe = one serving.** Servings scaling lives entirely in the logging path
  (`quantity * servings`) and in display, never in the recipe definition.
- **Deletion asymmetry / referential rules:**
  - food used by a recipe → cannot delete (`RESTRICT`).
  - food deleted but used in logs → `food_id` nulled, snapshot kept.
  - recipe deleted → ingredient rows cascade; logged entries untouched (soft ref).
  - user deleted → log entries + metrics cascade.
- **No auth / multi-tenancy on the catalog.** `foods` and `recipes` are global and
  shared across all users; only `log_entries` and `daily_metrics` are per-user.
  Consequently the food re-stamp on edit affects every user's history.
- **Dates are strings** (`YYYY-MM-DD`), validated by regex; "today" on the server is
  SQLite `date('now')` (UTC), while the frontend uses local `new Date()` —
  a potential timezone edge at day boundaries.
- **Recent window is fixed** at 7 days / 20 items via constants in `log.go`.
```
