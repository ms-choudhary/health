# Plan: Make Recipes the Only Way to Log Food

## Goal & mental model

Shift the product model so that:

- **Food = ingredient.** A food is a pure nutrition template (`kcal`/`protein` per
  unit). You build recipes out of foods. You **cannot** log a food directly.
- **Recipe = the only loggable unit.** Every entry in a user's daily log comes from
  logging a recipe at some number of servings.

Today there are **two** logging paths (`AddLogEntry` for single foods, `LogRecipe`
for recipes). This change **removes the single-food path** end-to-end and makes
`LogRecipe` the sole entry point, while keeping foods fully functional as recipe
ingredients.

> See `research.md` for the full current-state model. This plan assumes that
> understanding.

### Confirmed decisions (from review)

These choices are settled and folded into the plan below:

- **Rename** endpoint `/api/users/{id}/recent-foods` → **`/recent`**; handler
  `GetRecentFoods` → **`GetRecentRecipes`**; frontend `api.recentFoods` →
  **`api.recentRecipes`**.
- **Remove** the `Kind` discriminator everywhere — `recentItem` (Go) and
  `RecentItem` / `Pickable` (TS). The recent list is homogeneous (recipes only), so
  the tag carries no information.
- **Remove** the dead sqlc query `GetRecentLoggedFoods` and **regenerate** (no
  leave-it-in-place shortcut).
- **Rename** `AddFoodDrawer.vue` → **`AddRecipeDrawer.vue`**.
- **Relabel** the Library "Foods" tab → **"Ingredients"**.
- **Keep legacy single-food log entries; no migration.** Because of this, the
  `single` render branch + `removeEntry` in `UserPage.vue` **stay**. (The
  "delete the legacy branch after a DB reset" idea is intentionally **not** done —
  it conflicts with keeping legacy data.)

---

## 1. What stays, what changes, what's removed

### Stays unchanged
- **Foods CRUD** (`/api/foods`, `FoodEditor.vue`) — foods are still
  created/edited/deleted as ingredients (the tab is relabeled; behavior unchanged).
- **Recipes CRUD** (`/api/recipes`, `RecipeEditor.vue`, Library "Recipes" tab).
- **`LogRecipe`** handler + `POST /api/users/{id}/log/recipe`.
- **`GetLogForDate`** read path and recipe **delete-by-group** path.
- **Food re-stamp on edit** (`UpdateFood` → `RestampLogEntriesForFood`) — still
  relevant, because recipe-derived log rows carry `food_id`.
- The sqlc-generated **`AddLogEntry` DB method** — ⚠️ still used internally by
  `LogRecipe`. Only the *HTTP handler* of the same name is removed.
- Legacy single-food log rows (`source_recipe_id = NULL`) — kept and still rendered.

### Changes
- **Recent endpoint** renamed to `/recent` and returns **recipes only** (drop the
  foods half + the `Kind` tag).
- **Add-to-log drawer** (renamed `AddRecipeDrawer.vue`) searches/shows **recipes
  only**.
- **Library** Foods tab relabeled **"Ingredients."**
- **Seed** logs recipes instead of individual foods.

### Removed
- HTTP handler **`AddLogEntry`** + `logFoodBody` (`handlers/log.go`).
- Route registration `POST /api/users/{id}/log`.
- sqlc query **`GetRecentLoggedFoods`** (+ regenerate).
- The **`Kind`** field on `recentItem` (Go) and the `kind` discriminant on
  `RecentItem` / `Pickable` (TS).
- Frontend `api.addLog`, `AddLogPayload`, the `Pickable` union, food branches in the
  drawer.

---

## 2. Backend changes

### 2.1 Remove the single-food route + rename the recent route — `backend/main.go`

```go
// main.go

// DELETE this line (around line 51):
mux.HandleFunc("POST /api/users/{id}/log", h.AddLogEntry)

// RENAME the recent route + handler:
//   was: GET /api/users/{id}/recent-foods  -> h.GetRecentFoods
mux.HandleFunc("GET /api/users/{id}/recent", h.GetRecentRecipes)

// KEEP these unchanged:
mux.HandleFunc("GET /api/users/{id}/log", h.GetLog)
mux.HandleFunc("DELETE /api/users/{id}/log/recipe", h.DeleteLogEntriesByRecipe)
mux.HandleFunc("POST /api/users/{id}/log/recipe", h.LogRecipe)
mux.HandleFunc("DELETE /api/users/{id}/log/{eid}", h.DeleteLogEntry)
```

Leaving `GET .../log` registered but not `POST` removes the single-food write. In
practice `POST .../log` returns **404** with body `{"error":"not found"}` — the SPA
catch-all (`mux.Handle("/", spaHandler(...))`) also matches the path, so the mux
routes the request there rather than returning a bare 405, and `spaHandler` emits
the JSON 404 for `/api/` paths. Either way the operation is gone and creates no
entry. (Without the SPA fallback, Go 1.22 `ServeMux` would return 405 here.)

> Keep `DELETE /api/users/{id}/log/{eid}` so legacy single entries remain deletable.

### 2.2 Delete the single-food handler — `backend/handlers/log.go`

Remove the `logFoodBody` struct and the entire `AddLogEntry` **handler** func
(lines ~42–100). Do **not** touch anything that calls the sqlc `AddLogEntry`.

```go
// DELETE this whole block from handlers/log.go:
type logFoodBody struct {
	FoodID   *int64  `json:"food_id"`
	Quantity float64 `json:"quantity"`
	Date     string  `json:"date"`
}

func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
	// ... entire body ...
}
```

After deletion, double-check imports still used in `log.go` (`errors`, `sql`,
`time`, `sort`, `strconv` are still needed by the remaining handlers
`GetLog`, `DeleteLogEntry`, `DeleteLogEntriesByRecipe`, `GetRecentRecipes`).

### 2.3 Recent items → recipes only, no `Kind`, renamed handler — `backend/handlers/log.go`

Trim the handler so it no longer queries or maps foods, drop the `Kind` field, and
rename `GetRecentFoods` → `GetRecentRecipes`.

```go
// handlers/log.go

type recentItem struct {
	RecipeID      *int64  `json:"recipe_id"`
	RecipeName    string  `json:"recipe_name"`
	TotalCalories float64 `json:"total_calories"`
	TotalProtein  float64 `json:"total_protein"`
	LastServings  float64 `json:"last_servings"`
	maxID         int64
}

func (h *Handler) GetRecentRecipes(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	floor := time.Now().AddDate(0, 0, -recentWindowDays).Format("2006-01-02")

	recipes, err := h.Q.GetRecentLoggedRecipes(r.Context(), queries.GetRecentLoggedRecipesParams{
		UserID:    userID,
		DateFloor: floor,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]recentItem, 0, len(recipes))
	for _, rec := range recipes {
		rid := rec.RecipeID
		items = append(items, recentItem{
			RecipeID:      &rid,
			RecipeName:    rec.RecipeName,
			TotalCalories: rec.TotalCalories,
			TotalProtein:  rec.TotalProtein,
			LastServings:  rec.LastServings,
			maxID:         rec.MaxID,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].maxID > items[j].maxID })
	if len(items) > recentItemsCap {
		items = items[:recentItemsCap]
	}
	writeJSON(w, http.StatusOK, items)
}
```

> Dropped the `,omitempty` on `recipe_id`/`recipe_name` since every item now always
> has them — they're no longer optional.

### 2.4 Remove the dead query + regenerate sqlc — `backend/sql/queries/log.sql`

Delete the `GetRecentLoggedFoods` query block (lines ~6–26). **Keep** `AddLogEntry`,
`GetRecentLoggedRecipes`, `GetLogForDate`, `DeleteLogEntry`,
`DeleteLogEntriesByRecipe`, `SumNutritionByDateRange`.

Then regenerate the typed DB layer:

```bash
cd backend
sqlc generate          # sqlc is at /home/agent/go/bin/sqlc
go build ./...         # confirm GetRecentLoggedFoods / its Row type are gone & nothing references them
```

This removes `GetRecentLoggedFoods`, `GetRecentLoggedFoodsParams`, and
`GetRecentLoggedFoodsRow` from `db/queries/log.sql.go`. If `go build` complains
about a leftover reference, you missed a caller — fix and rebuild.

### 2.5 Update the seed — `backend/cmd/seed/main.go`

Today the seed inserts **single-food** log entries (`main.go:94`, `source_recipe_id`
NULL). That contradicts the new model (those rows could no longer be created via the
app). Replace the per-day single-food loop with **recipe logging**, mirroring
`LogRecipe`'s explosion (one row per ingredient, tagged with `source_recipe_*`).

```go
// after recipes + ingredients are created, build a lookup of recipe -> ingredients
// (reuse the createdRecipes list; fetch ingredients via q.GetRecipeIngredients).

for _, u := range createdUsers {
	baseWeight := 70.0 + rng.Float64()*15
	for d := 13; d >= 0; d-- {
		date := today.AddDate(0, 0, -d).Format("2006-01-02")
		// ... UpsertMetrics unchanged ...

		mealsPerDay := 2 + rng.Intn(2)
		for i := 0; i < mealsPerDay; i++ {
			recipe := createdRecipeRows[rng.Intn(len(createdRecipeRows))]
			ings, err := q.GetRecipeIngredients(ctx, recipe.ID)
			if err != nil || len(ings) == 0 {
				continue
			}
			servings := 1.0 + float64(rng.Intn(2)) // 1 or 2 servings
			for _, ing := range ings {
				qty := ing.Quantity * servings
				rid, rname := recipe.ID, recipe.Name
				if _, err := q.AddLogEntry(ctx, queries.AddLogEntryParams{
					UserID: u.ID, FoodID: &ing.FoodID, Date: date,
					FoodName: ing.FoodName, FoodUnit: ing.FoodUnit,
					CaloriesPerUnit: ing.CaloriesPerUnit, ProteinPerUnit: ing.ProteinPerUnit,
					Quantity: qty,
					Calories: ing.CaloriesPerUnit * qty, Protein: ing.ProteinPerUnit * qty,
					SourceRecipeID: &rid, SourceRecipeName: &rname, SourceRecipeServings: &servings,
				}); err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}
```

This requires moving recipe creation **above** the day loop and keeping the created
`queries.Recipe` rows (e.g. `createdRecipeRows`). The standalone `createdFoods` log
loop is removed.

---

## 3. Frontend changes

### 3.1 Types — `frontend/src/lib/types.ts`

Collapse `RecentItem` to a plain recipe interface (no `kind`), remove the `Pickable`
union, and delete `AddLogPayload`.

```ts
// RecentItem: recipe-only, no discriminant.
export interface RecentItem {
  recipe_id: number
  recipe_name: string
  total_calories: number
  total_protein: number
  last_servings: number
}

// DELETE the Pickable union entirely — the drawer uses RecipeListItem directly.
// DELETE AddLogPayload entirely — no longer used.
```

### 3.2 API client — `frontend/src/lib/api.ts`

```ts
// DELETE:
addLog: (userId, payload: AddLogPayload) => ...

// RENAME recentFoods -> recentRecipes, and point it at /recent:
recentRecipes: (userId: number) =>
  request<RecentItem[]>(`${BASE}/users/${userId}/recent`),

// KEEP logRecipe, deleteLog (legacy singles), deleteLogRecipeGroup,
// listFoods (still used by RecipeEditor + Library), listRecipes, getRecipe.
```

Remove the now-unused `AddLogPayload` import. Update the one caller of
`api.recentFoods` (the drawer) to `api.recentRecipes`.

### 3.3 Add-to-log drawer — rename to `frontend/src/components/AddRecipeDrawer.vue`

Rename the file/component `AddFoodDrawer.vue` → **`AddRecipeDrawer.vue`** and update
its import in `UserPage.vue` (§3.4). The drawer becomes recipe-only.

**Script changes:**

```ts
import type { RecentItem, RecipeListItem } from '@/lib/types'

// Single picked shape, no discriminant.
interface Picked {
  recipe_id: number
  recipe_name: string
  total_calories: number
  total_protein: number
  scale: string
}

const recent = ref<RecentItem[]>([])
const results = ref<RecipeListItem[]>([])   // was Pickable[]
const picked = ref<Picked | null>(null)

const emit = defineEmits<{
  close: []
  added: [payload: { recipe_id: number }]   // no more AddLogPayload
}>()

// previewCalories / previewProtein: recipe math only.
const previewCalories = computed<number>(() => {
  if (!picked.value) return 0
  const n = Number(picked.value.scale)
  if (!Number.isFinite(n) || n <= 0) return 0
  return n * picked.value.total_calories
})
// previewProtein analogous with total_protein.

// Search: recipes only, no mapping into a union.
watch(query, (v) => {
  window.clearTimeout(searchTimer)
  const trimmed = v.trim()
  if (!trimmed) { results.value = []; return }
  searchTimer = window.setTimeout(async () => {
    results.value = await api.listRecipes(trimmed)
  }, 200)
})

// recent is recipe-only now (no kind check).
async function loadRecent(): Promise<void> {
  try {
    recent.value = await api.recentRecipes(props.userId)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load recent recipes'
  }
}

function pickRecent(item: RecentItem): void {
  picked.value = {
    recipe_id: item.recipe_id,
    recipe_name: item.recipe_name,
    total_calories: item.total_calories,
    total_protein: item.total_protein,
    scale: String(item.last_servings),
  }
}

function pickRecipe(recipe: RecipeListItem): void {
  picked.value = {
    recipe_id: recipe.id,
    recipe_name: recipe.name,
    total_calories: recipe.total_calories,
    total_protein: recipe.total_protein,
    scale: '1',
  }
}

// confirm(): logRecipe only.
async function confirm(): Promise<void> {
  if (!picked.value) return
  const servings = Number(picked.value.scale)
  if (!Number.isFinite(servings) || servings <= 0) {
    errMsg.value = 'Servings must be a positive number'
    return
  }
  saving.value = true; errMsg.value = ''
  const recipeId = picked.value.recipe_id
  try {
    await api.logRecipe(props.userId, { recipe_id: recipeId, servings, date: props.date })
    emit('added', { recipe_id: recipeId })
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to log recipe'
  } finally {
    saving.value = false
  }
}
```

**Template changes:**
- Remove the `picked.kind === 'food'` `<template>` branch (quantity input block);
  keep only the servings/scale block (now unconditional).
- Recent list + Library results: drop the `item.kind` branching entirely. Recent
  iterates `RecentItem[]` (all recipes); results iterate `RecipeListItem[]`
  directly (`item.name`, `item.total_calories`, …) calling `pickRecipe(item)`.
- Update copy: header can stay "Add to log"; search placeholder →
  `"Search recipes…"`; empty states →
  `"No recipes yet — create one in the Library first."`

### 3.4 Log view — `frontend/src/views/UserPage.vue`

- Update the import + tag: `AddFoodDrawer` → `AddRecipeDrawer`.
- **Keep** the `groups` computed and the `single` rendering branch **for legacy
  back-compat** (entries with `source_recipe_id === null` predate this change).
  Keep `removeEntry` for those. *(Per the decision to keep legacy data, this branch
  is not removed.)*
- Update the "Add food" button label → **"Add recipe"**:

```vue
<Button variant="outline" @click="showDrawer = true">
  <Plus class="h-4 w-4" />
  Add recipe
</Button>
```

- `onFoodAdded` and the drawer's `@added` handler need no logic change (payload is
  now always `{ recipe_id }`); optionally rename `onFoodAdded` → `onRecipeAdded`.

### 3.5 Library — relabel Foods tab to "Ingredients" — `frontend/src/views/FoodLibrary.vue`

Reinforce the model: the Foods tab becomes **"Ingredients."** This is a label change
only — the underlying `tab` value, `/api/foods` calls, and `FoodEditor` stay as-is.

```vue
<Button :variant="tab === 'foods' ? 'default' : 'outline'" size="sm" @click="tab = 'foods'">
  Ingredients
</Button>
```

Optionally also update the empty-state copy ("No ingredients yet — tap 'Add
ingredient' below.") and the "Add food" button label → "Add ingredient" for
consistency. The internal `Tab` type / `'foods'` key can stay to minimize churn.

---

## 4. Data & migration considerations

1. **Legacy single-food entries — kept, no migration (decided).** Existing rows have
   `source_recipe_id = NULL`. The read path returns them; `UserPage.groups` still
   renders them as `single`; they remain deletable via `removeEntry`. No new singles
   are ever created. No DB migration is run.
2. **`source_recipe_id` is not a FK** — unchanged. Logged entries still survive
   recipe deletion.
3. **Food re-stamp still correct** — recipe-derived rows carry `food_id`, so editing
   a food still updates historical logged rows and live recipe totals consistently.
4. **No DB schema change is required.** `log_entries` already supports both shapes;
   we're just narrowing which shape gets written.

---

## 5. Implementation order (suggested)

1. **Backend handler/route** removal + recent rename (§2.1, §2.2, §2.3) — compile.
2. **SQL query removal + `sqlc generate`** (§2.4) — `go build ./...`.
3. **Seed** update (§2.5) — `go run ./cmd/seed` against a scratch DB to verify.
4. **Frontend types + api** (§3.1, §3.2) — `tsc` / `npm run build`.
5. **Drawer** rename + rewrite (§3.3).
6. **UserPage** import/label + back-compat check (§3.4).
7. **Library** tab relabel (§3.5).
8. Manual verification (§6) + tests (§7).

Do backend first so the frontend builds against the final API shape (including the
renamed `/recent` route).

---

## 6. Manual verification checklist

- [ ] `POST /api/users/{id}/log` returns **404** `{"error":"not found"}` (route
      gone, SPA fallback handles it) and creates no entry; `POST .../log/recipe`
      still works.
- [ ] `GET /api/users/{id}/recent` returns recipe-only items with **no `kind`
      field**, capped at 20, ordered by recency; the old `/recent-foods` 404s.
- [ ] Add-to-log drawer (now `AddRecipeDrawer`): search + Recent show **only
      recipes**; logging a recipe at N servings creates N×(#ingredients) rows.
- [ ] A logged recipe renders as a grouped header + indented ingredients; group
      delete removes all its rows for that day only.
- [ ] Editing a food's nutrition still re-stamps historical recipe-logged rows.
- [ ] Library "Ingredients" tab works; foods & recipes CRUD unaffected; deleting a
      food used by a recipe is still blocked.
- [ ] Legacy single entries still display and delete correctly.
- [ ] `today` summary + progress charts still total correctly.

## 7. Tests (recommended — none exist today)

Add table-driven Go handler tests (the repo currently has **no** `*_test.go`):

- `LogRecipe`: happy path (row count = ingredients × servings, correct
  `source_recipe_*` + scaled calories), `servings <= 0`, missing recipe (404),
  recipe with no ingredients (400).
- `GetRecentRecipes`: returns recipe-only items (no `kind`), capped at 20, ordered
  by recency.
- Route check: `POST /api/users/{id}/log` → 405.

---

## 8. Rollback

The change is additive-removal with no schema migration, so rollback is a `git
revert` of the diff. Because `log_entries` is unchanged and legacy singles were
preserved (no destructive migration), reverting restores the single-food path with
zero data loss.
