# Plan: refactor `addLogBody` + unify Recent (foods & recipes)

Two related changes:

1. **Refactor `addLogBody` → `logFoodBody`** — narrow the wire format to
   `{food_id, quantity, date}`. The backend resolves food name / unit /
   per-unit calories / per-unit protein from the `foods` table at log time
   instead of trusting the client to send them.

2. **Unify Recent** — `GET /api/users/{id}/recent-foods` should also
   return recipes the user has recently logged. The Add-to-log drawer's
   "Recent" section becomes a deduped, time-windowed mix of foods and
   recipes that the user can re-log with one tap.

Part 1 is a prerequisite for Part 2 only in spirit (both touch the log
flow); they can ship together or one-after-the-other.

---

## Part 1 — Refactor `addLogBody` → `logFoodBody`

### Goal

Today the client tells the server everything about the food being logged:
name, unit, kcal/unit, protein/unit. The server just multiplies by
quantity and stores. This is a holdover from the snapshot design; with
`RestampLogEntriesForFood` already keeping snapshots in sync with food
edits, there's no reason the client should be the source of truth for
those values.

Target body shape:

```go
type logFoodBody struct {
    FoodID   *int64  `json:"food_id"`
    Quantity float64 `json:"quantity"`
    Date     string  `json:"date"`
}
```

(Pointer for parity with `logRecipeBody`'s style, but a `food_id` of nil
or 0 is rejected — it's effectively required.)

### Implications & decisions

- **`food_id` becomes required.** The current API allowed `food_id: null`,
  which let users re-log a food whose library entry had been deleted (the
  client still had the snapshot). After this refactor, the server needs a
  live food row to read values from, so deleted foods can no longer be
  re-logged. Recent will simply filter them out (see Part 2's foods
  query).
- **Calories/protein are computed server-side.** Removes a trust boundary
  — clients can't lie about kcal/protein per unit anymore.
- **AI-suggested foods unaffected.** They go through `CreateFood` first in
  `FoodLibrary.vue`, then the regular log flow — still works.

### Backend changes

#### `backend/handlers/log.go`

Replace the struct and rewrite `AddLogEntry`:

```go
type logFoodBody struct {
    FoodID   *int64  `json:"food_id"`
    Quantity float64 `json:"quantity"`
    Date     string  `json:"date"`
}

func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
    userID, err := parseID(r, "id")
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    var body logFoodBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if body.FoodID == nil || *body.FoodID <= 0 {
        writeError(w, http.StatusBadRequest, "food_id required")
        return
    }
    if body.Quantity <= 0 {
        writeError(w, http.StatusBadRequest, "quantity must be > 0")
        return
    }
    if !validDate(body.Date) {
        writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
        return
    }
    food, err := h.Q.GetFood(r.Context(), *body.FoodID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeError(w, http.StatusNotFound, "food not found")
            return
        }
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    entry, err := h.Q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
        UserID:               userID,
        FoodID:               &food.ID,
        Date:                 body.Date,
        FoodName:             food.Name,
        FoodUnit:             food.Unit,
        CaloriesPerUnit:      food.CaloriesPerUnit,
        ProteinPerUnit:       food.ProteinPerUnit,
        Quantity:             body.Quantity,
        Calories:             food.CaloriesPerUnit * body.Quantity,
        Protein:              food.ProteinPerUnit * body.Quantity,
        SourceRecipeID:       nil,
        SourceRecipeName:     nil,
        SourceRecipeServings: nil,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    writeJSON(w, http.StatusCreated, entry)
}
```

Imports to add: `database/sql`, `errors`.

`GetFood` already exists in `sql/queries/foods.sql` (`SELECT * FROM foods
WHERE id = ?`). No new SQL needed.

#### Nothing else on the backend needs to change for Part 1.

### Frontend changes

#### `frontend/src/lib/types.ts`

```ts
export interface AddLogPayload {
  food_id: number
  quantity: number
  date: string
}
```

(was 6 fields, now 3 — `food_id` is required, so drop the `| null`.)

#### `frontend/src/components/AddFoodDrawer.vue`

The `PickedFood` interface in the component can be trimmed too, since the
extra snapshot fields are no longer needed for the POST:

```ts
interface PickedFood {
  kind: 'food'
  food_id: number          // was: number | null
  food_name: string         // keep — used for the preview UI
  food_unit: string         // keep — used for the preview UI
  calories_per_unit: number // keep — used for the live kcal preview
  protein_per_unit: number  // keep — used for the live protein preview
  quantity: string
}
```

`pickRecent` and `pickLibraryFood` keep populating these fields from
their respective sources; they're now strictly client-side UI state.

`confirm` builds the slimmer payload:

```ts
const payload: AddLogPayload = {
  food_id: picked.value.food_id,
  quantity: n,
  date: props.date,
}
await api.addLog(props.userId, payload)
```

If `food_id` is `null` at pick time (currently possible from a Recent
item whose source food was deleted), `pickRecent` should reject the
selection — but Part 2's query will filter those rows out before they
reach the client, so this is belt-and-braces. Easiest path: make the
type `number` (not `number | null`) on `PickedFood`, and rely on the
Recent backend filter.

### Tests / smoke

- Open the drawer, pick a Recent food, log it → succeeds.
- Open the drawer, search a food, log it → succeeds.
- Log a food, then edit that food's calories in the library, then log
  it again on the same day → second entry uses the new value (and the
  first entry also reflects the new value, because of the existing
  `RestampLogEntriesForFood`).
- Try POSTing without `food_id` → 400.
- Try POSTing with a `food_id` that doesn't exist → 404.

---

## Part 2 — Recipes in Recent

### Goal

Recent today is foods-only. We want a single ordered list — most-recent
first — that includes both individual foods the user has logged and
recipes the user has logged, deduplicated, restricted to "the last
couple of days" (proposed: 7 days), capped at a reasonable count.

UX: the Add-to-log drawer's "Recent" section becomes a mixed list where
each row is either a food (tap → quantity input, prefilled with last
quantity) or a recipe (tap → servings input, prefilled with last
servings).

### API shape

`GET /api/users/{id}/recent-foods` (we'll keep the path for minimal
churn, though `/recent` would be more honest — call it out in PR
description, decide later).

Response: a discriminated array.

```ts
export type RecentItem =
  | {
      kind: 'food'
      food_id: number
      food_name: string
      food_unit: string
      calories_per_unit: number
      protein_per_unit: number
      last_quantity: number
    }
  | {
      kind: 'recipe'
      recipe_id: number
      recipe_name: string
      total_calories: number
      total_protein: number
      last_servings: number
    }
```

Server emits each row with a `kind` discriminator. We can drop the
`max_id` from the wire — ordering is server-side.

### Backend changes

#### Date floor

Proposed: last 7 days, computed at request time:

```go
floor := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
```

Pulled into a constant or `const recentWindowDays = 7` near the top of
the handler. Alternative: accept `?days=N` as an optional query param
(capped to a sane max). For simplicity, hard-code first and add the
param later if asked.

#### SQL: `sql/queries/log.sql`

Tweak the existing foods query (filter by date + non-null food_id, and
also group/identify by `food_id` rather than `food_name` so library
renames don't split history):

```sql
-- name: GetRecentLoggedFoods :many
SELECT
  le.food_id,
  le.food_name,
  le.food_unit,
  le.calories_per_unit,
  le.protein_per_unit,
  le.quantity AS last_quantity,
  latest.max_id AS max_id
FROM log_entries le
INNER JOIN (
  SELECT inner_le.food_id AS fid, MAX(inner_le.id) AS max_id
  FROM log_entries inner_le
  WHERE inner_le.user_id        = sqlc.arg(user_id)
    AND inner_le.source_recipe_id IS NULL
    AND inner_le.food_id        IS NOT NULL
    AND inner_le.date           >= sqlc.arg(date_floor)
  GROUP BY inner_le.food_id
) latest ON le.id = latest.max_id
ORDER BY le.id DESC
LIMIT 20;
```

Add a new query for recipes:

```sql
-- name: GetRecentLoggedRecipes :many
SELECT
  r.id   AS recipe_id,
  r.name AS recipe_name,
  COALESCE(le.source_recipe_servings, 1) AS last_servings,
  CAST(COALESCE(SUM(f.calories_per_unit * ri.quantity), 0) AS REAL) AS total_calories,
  CAST(COALESCE(SUM(f.protein_per_unit  * ri.quantity), 0) AS REAL) AS total_protein,
  latest.max_id AS max_id
FROM (
  SELECT source_recipe_id AS rid, MAX(id) AS max_id
  FROM log_entries
  WHERE user_id           = sqlc.arg(user_id)
    AND source_recipe_id IS NOT NULL
    AND date             >= sqlc.arg(date_floor)
  GROUP BY source_recipe_id
) latest
JOIN log_entries le ON le.id = latest.max_id
JOIN recipes      r  ON r.id = latest.rid
LEFT JOIN recipe_ingredients ri ON ri.recipe_id = r.id
LEFT JOIN foods              f  ON f.id          = ri.food_id
GROUP BY r.id, le.source_recipe_servings, latest.max_id
ORDER BY latest.max_id DESC
LIMIT 10;
```

Key design notes on the recipe query:

- The inner subquery picks `MAX(id)` per `source_recipe_id` within the
  date window — one row per recipe the user logged in the window.
- Outer join to `log_entries` pulls the snapshotted `source_recipe_servings`
  from that latest log instance (for prefilling the scale input).
- `JOIN recipes` (not `LEFT JOIN`) hides recipes that have been deleted
  — we can't re-log them anyway (the `LogRecipe` handler needs live
  ingredients).
- `LEFT JOIN recipe_ingredients / foods` lets us compute current
  total_calories / total_protein per serving, mirroring how
  `ListRecipes` does it. This is "fresh" data, not snapshot — if the
  user updated the recipe since last logging it, the Recent row shows
  the new totals. That seems right: they're about to log it again with
  current ingredients.
- `COALESCE(le.source_recipe_servings, 1)` covers legacy rows logged
  before `source_recipe_servings` existed; they default to 1 serving.

Run `sqlc generate` to regenerate:
- `db/queries/log.sql.go`: updates `GetRecentLoggedFoodsParams` (now
  takes `UserID` + `DateFloor`), updates the row struct (adds
  `FoodID *int64` non-null, drops nothing, adds `MaxID int64`), and
  introduces `GetRecentLoggedRecipes`, `GetRecentLoggedRecipesParams`,
  and `GetRecentLoggedRecipesRow`.

#### Handler: `backend/handlers/log.go`

```go
type recentItem struct {
    Kind string `json:"kind"`
    // food fields (kind == "food")
    FoodID          *int64  `json:"food_id,omitempty"`
    FoodName        string  `json:"food_name,omitempty"`
    FoodUnit        string  `json:"food_unit,omitempty"`
    CaloriesPerUnit float64 `json:"calories_per_unit,omitempty"`
    ProteinPerUnit  float64 `json:"protein_per_unit,omitempty"`
    LastQuantity    float64 `json:"last_quantity,omitempty"`
    // recipe fields (kind == "recipe")
    RecipeID      *int64  `json:"recipe_id,omitempty"`
    RecipeName    string  `json:"recipe_name,omitempty"`
    TotalCalories float64 `json:"total_calories,omitempty"`
    TotalProtein  float64 `json:"total_protein,omitempty"`
    LastServings  float64 `json:"last_servings,omitempty"`
    // sorting key (not serialised; field-only)
    maxID int64
}

const recentWindowDays = 7

func (h *Handler) GetRecentFoods(w http.ResponseWriter, r *http.Request) {
    userID, err := parseID(r, "id")
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    floor := time.Now().AddDate(0, 0, -recentWindowDays).Format("2006-01-02")

    foods, err := h.Q.GetRecentLoggedFoods(r.Context(), queries.GetRecentLoggedFoodsParams{
        UserID:    userID,
        DateFloor: floor,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    recipes, err := h.Q.GetRecentLoggedRecipes(r.Context(), queries.GetRecentLoggedRecipesParams{
        UserID:    userID,
        DateFloor: floor,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }

    items := make([]recentItem, 0, len(foods)+len(recipes))
    for _, f := range foods {
        items = append(items, recentItem{
            Kind:            "food",
            FoodID:          f.FoodID,
            FoodName:        f.FoodName,
            FoodUnit:        f.FoodUnit,
            CaloriesPerUnit: f.CaloriesPerUnit,
            ProteinPerUnit:  f.ProteinPerUnit,
            LastQuantity:    f.LastQuantity,
            maxID:           f.MaxID,
        })
    }
    for _, rec := range recipes {
        rid := rec.RecipeID
        items = append(items, recentItem{
            Kind:          "recipe",
            RecipeID:      &rid,
            RecipeName:    rec.RecipeName,
            TotalCalories: rec.TotalCalories,
            TotalProtein:  rec.TotalProtein,
            LastServings:  rec.LastServings,
            maxID:         rec.MaxID,
        })
    }
    sort.Slice(items, func(i, j int) bool { return items[i].maxID > items[j].maxID })

    // Optional: cap the merged list (e.g. 20) so a chatty user doesn't
    // get a 30-item drawer.
    if len(items) > 20 {
        items = items[:20]
    }

    writeJSON(w, http.StatusOK, items)
}
```

Add imports: `sort`, `time`, `health/db/queries`.

Note on the `maxID` field: it's an unexported struct field, so
`encoding/json` skips it. If you'd rather not use this trick, sort
into a `sort.Interface` wrapper before mapping into the response
shape.

### Frontend changes

#### `frontend/src/lib/types.ts`

```ts
export type RecentItem =
  | {
      kind: 'food'
      food_id: number
      food_name: string
      food_unit: string
      calories_per_unit: number
      protein_per_unit: number
      last_quantity: number
    }
  | {
      kind: 'recipe'
      recipe_id: number
      recipe_name: string
      total_calories: number
      total_protein: number
      last_servings: number
    }
```

Delete the old `RecentFood` interface; update the imports.

#### `frontend/src/lib/api.ts`

```ts
recentFoods: (userId: number) =>
  request<RecentItem[]>(`${BASE}/users/${userId}/recent-foods`),
```

(Type changes, name stays — minimal churn. Rename the method to
`recentItems` later if desired.)

#### `frontend/src/components/AddFoodDrawer.vue`

State:

```ts
const recent = ref<RecentItem[]>([])
```

`pickRecent` becomes branching:

```ts
function pickRecent(item: RecentItem): void {
  if (item.kind === 'food') {
    picked.value = {
      kind: 'food',
      food_id: item.food_id,
      food_name: item.food_name,
      food_unit: item.food_unit,
      calories_per_unit: item.calories_per_unit,
      protein_per_unit: item.protein_per_unit,
      quantity: String(item.last_quantity),
    }
  } else {
    picked.value = {
      kind: 'recipe',
      recipe_id: item.recipe_id,
      recipe_name: item.recipe_name,
      total_calories: item.total_calories,
      total_protein: item.total_protein,
      scale: String(item.last_servings),  // prefilled from last log
    }
  }
}
```

Template: render each row differently based on `kind`:

```vue
<template v-for="item in recent">
  <button
    v-if="item.kind === 'food'"
    :key="`f-${item.food_id}`"
    type="button"
    class="text-left p-3 rounded-lg border border-border …"
    @click="pickRecent(item)"
  >
    <div class="min-w-0">
      <div class="font-medium truncate">{{ item.food_name }}</div>
      <div class="text-xs text-muted-foreground">
        {{ item.last_quantity }} {{ item.food_unit }} ·
        {{ Math.round(item.last_quantity * item.calories_per_unit) }} kcal last time
      </div>
    </div>
    <Badge variant="secondary">
      {{ Math.round(item.last_quantity * item.calories_per_unit) }}
    </Badge>
  </button>

  <button
    v-else
    :key="`r-${item.recipe_id}`"
    type="button"
    class="text-left p-3 rounded-lg border border-border …"
    @click="pickRecent(item)"
  >
    <div class="min-w-0">
      <div class="font-medium truncate">{{ item.recipe_name }}</div>
      <div class="text-xs text-muted-foreground">
        {{ formatNumber(item.last_servings, item.last_servings % 1 ? 1 : 0) }}
        serving{{ item.last_servings === 1 ? '' : 's' }} last time ·
        {{ Math.round(item.last_servings * item.total_calories) }} kcal
      </div>
    </div>
    <Badge variant="secondary">Recipe</Badge>
  </button>
</template>
```

Empty-state copy and the `:key`s update accordingly. The library
search list (the other branch of the drawer) is unchanged.

### Tests / smoke

- Log a food → reopen the drawer → it appears in Recent.
- Log a recipe → reopen the drawer → it appears in Recent with the
  "Recipe" badge and the last-used servings.
- Log a recipe with 1.5 servings → Recent shows "1.5 servings last
  time"; tapping prefills the scale input with `1.5`.
- Delete a food → reopen → the food disappears from Recent.
- Delete a recipe → reopen → the recipe disappears from Recent.
- Log a recipe 8 days ago, nothing since → Recent is empty (date
  floor filters it out).
- Mix of foods and recipes logged in the window → merged list is
  ordered newest-first by log id, regardless of food vs recipe kind.

---

## Migration / rollout

- **No DB schema change.** Both parts work with existing columns
  (`source_recipe_id`, `source_recipe_name`, `source_recipe_servings`
  are already in place; foods table already has everything needed).
- **Wire-incompatible change** on `POST /api/users/{id}/log`: old
  clients sending the full body still work *if* the backend ignores
  extra JSON fields (Go's `json.NewDecoder` does by default), but the
  required field flips from "everything" to just `food_id`. Since the
  frontend ships with the backend in one binary, this is a single-deploy
  swap — no version-skew risk.
- **Wire shape change** on `GET /api/users/{id}/recent-foods`: response
  type changes from `RecentFood[]` to `RecentItem[]`. Same single-deploy
  swap.

## Order of operations

Recommended order, each a separate commit so review is easy:

1. **Part 1 backend** — refactor `addLogBody` → `logFoodBody`,
   handler reads `foods.GetFood`. Smoke-test with the unchanged
   frontend (it still sends the old body; Go decodes the subset and
   uses just `food_id` + `quantity` + `date`). The extra fields the
   client sends are ignored.
2. **Part 1 frontend** — trim `AddLogPayload`, send only the three
   fields.
3. **Part 2 backend** — update foods SQL (window + food_id grouping),
   add recipes SQL, regenerate sqlc, rewrite `GetRecentFoods` handler
   to merge.
4. **Part 2 frontend** — `RecentItem` discriminated union,
   `AddFoodDrawer` template branching on `kind`.

Each step builds and runs end-to-end on its own.

---

## Risks & things to verify

- **Order in Recent** — sorting by `max_id` desc means a back-dated
  food log will jump to the top of Recent. Probably fine (consistent
  with the existing query) but worth eyeballing.
- **Recipe totals freshness** — Recent shows *current* recipe totals
  (joined to `recipes/recipe_ingredients/foods`), not what was logged.
  If a user logged a 200-kcal recipe last week and you've since changed
  one ingredient to make it 350 kcal, Recent will show 350. This
  matches the "you're about to log it again with the new definition"
  intent.
- **Servings prefill correctness** — depends on `source_recipe_servings`
  having been backfilled or written from the start. Legacy log rows
  from before that column existed have NULL; the `COALESCE(..., 1)`
  defaults them to 1.
- **`Recipe` badge styling** — the existing "Recipe" badge in the
  library-search branch uses `Badge variant="secondary"`. Reuse the
  same look in Recent for consistency.
- **`item.last_servings === 1` strict equality** in the template — if
  the prefill ends up as `1.0` (`number`), strict-equality with `1`
  still matches. If it ever arrives as a string, it won't; keep the
  type strict in the discriminated union.
