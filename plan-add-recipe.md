# Plan: Log a custom (ad-hoc) recipe from the Food log

## Goal

Today, the **Add** button on the **Log → Food** tab opens `AddRecipeDrawer`, which lets the
user pick a recent/searched recipe to log. We are **replacing** that: the **Add** button will
open a **custom-recipe editor** and **nothing else** — no recent list, no recipe search. The
user builds a **one-off recipe** (a name plus a list of ingredients with quantities) and logs
it directly. The recipe is **not** persisted to the recipe library; it only produces log
entries for the chosen day.

Because recent-recipe picking is being removed from this surface, this plan also includes a
**deep cleanup** of everything that becomes redundant (see
[Deep cleanup](#deep-cleanup-remove-now-redundant-functionality)).

The custom-recipe UI is essentially a stripped-down `RecipeEditor`:
- Recipe **name**
- **Ingredients** picked from the ingredient library, each with a quantity
- Live **calories / protein** total
- **No** food tags, **no** "save recipe" — instead a single **"Log recipe"** action

---

## Why this is cheap to build

Two facts about the current data model make this almost free:

1. **Log rows are fully denormalized.** Each `log_entries` row already stores its own
   `ingredient_name`, `ingredient_unit`, `calories_per_unit`, `protein_per_unit`,
   `quantity`, `calories`, `protein` (`backend/db/schema.sql:19-34`). A log row does **not**
   depend on a recipe or even an ingredient existing — `ingredient_id` is nullable.

2. **`source_recipe_id` is a plain integer column with no foreign key.** The Food log
   groups rows purely by `source_recipe_id` and labels each group with
   `source_recipe_name` (`frontend/src/components/LogTab.vue:49-85`). Deletion of a group is
   keyed on `(date, source_recipe_id)` (`LogTab.vue:125-131` →
   `backend/handlers/log.go:53-78`).

So a custom recipe is just **N denormalized log rows that share one synthetic group id and a
display name**. We reuse the existing grouping, totals, and delete logic untouched.

---

## Design decision: how to group ad-hoc rows

The Food log only renders rows where `source_recipe_id != null` and groups them by that id
(`LogTab.vue:52-53`). Custom recipes have no persisted recipe row, so they need a group id
that:

- is **non-null** (so the rows render),
- is **unique per logged recipe** (so two custom recipes don't merge),
- **never collides** with a real recipe id.

### Recommended: negative synthetic group id

Real recipe ids are positive auto-increment integers. We assign each custom recipe a
**negative** group id derived from a nanosecond timestamp (`-time.Now().UnixNano()`),
computed once per request and written to `source_recipe_id` on every row of that recipe.

Why this is the right tradeoff:

- **Reuses everything.** Grouping (`LogTab.vue:66`), per-group totals, the "Recipe" badge,
  and group deletion (`deleteLogRecipeGroup`) all work unchanged because they only care that
  the id is a stable non-null integer.
- **No collision with real recipes.** `TodayTab` still logs real (positive-id) recipes into
  the same log. A negative id guarantees a custom recipe can never accidentally merge with a
  real recipe group on the same day.
- **No schema migration.** We write to existing columns only.

The one small backend change this forces: the group-delete handler currently rejects any
`source_recipe_id <= 0` (`backend/handlers/log.go:64-67`). We relax that to reject only `0`.

### Alternative (rejected for now): dedicated column

Add a `source_label TEXT` / `ad_hoc_group_id INTEGER` column and change the frontend to
group on a composite key. Cleaner conceptually, but it touches the schema, the sqlc queries,
the grouping computed, and the delete endpoint — far more surface area for no user-visible
benefit. Note it as a future refactor if ad-hoc recipes become first-class.

---

## Design decision: payload shape

We make the new endpoint accept **fully-specified line items** rather than just
`ingredient_id`s. Because log rows are denormalized, the backend doesn't need to look the
ingredients up — the client already has name/unit/cal/protein from the library picker. This
also keeps the door open for free-form items later (typing a name + calories without a
library ingredient) with zero backend change.

```jsonc
// POST /api/users/{id}/log/custom-recipe
{
  "name": "Tuesday lunch",
  "date": "2026-06-15",
  "items": [
    {
      "ingredient_id": 12,          // optional, for traceability; may be null
      "ingredient_name": "Chicken breast",
      "ingredient_unit": "g",
      "calories_per_unit": 1.65,
      "protein_per_unit": 0.31,
      "quantity": 200
    }
  ]
}
```

There is **no servings multiplier** for custom recipes — the user enters absolute quantities,
so we store `source_recipe_servings = null` and the group header simply omits the
"N servings" line (`LogTab.vue:204`).

---

## Backend changes

### 1. Relax the group-delete validation

`backend/handlers/log.go` — allow negative group ids (only `0` is invalid):

```go
// DeleteLogEntriesByRecipe (currently lines ~64-67)
srid, err := strconv.ParseInt(r.URL.Query().Get("source_recipe_id"), 10, 64)
if err != nil || srid == 0 {            // was: srid <= 0
	writeError(w, http.StatusBadRequest, "source_recipe_id required")
	return
}
```

### 2. New handler: `LogCustomRecipe`

Named `LogCustomRecipe` to stay consistent with the existing `LogRecipe` handler. Add to
`backend/handlers/log.go`. It reuses the existing `AddLogEntry` query
(`backend/db/queries/log.sql.go:38`) — no new sqlc query required.

```go
type customRecipeItem struct {
	IngredientID    *int64  `json:"ingredient_id"`
	IngredientName  string  `json:"ingredient_name"`
	IngredientUnit  string  `json:"ingredient_unit"`
	CaloriesPerUnit float64 `json:"calories_per_unit"`
	ProteinPerUnit  float64 `json:"protein_per_unit"`
	Quantity        float64 `json:"quantity"`
}

type logCustomRecipeBody struct {
	Name  string             `json:"name"`
	Date  string             `json:"date"`
	Items []customRecipeItem `json:"items"`
}

func (h *Handler) LogCustomRecipe(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var body logCustomRecipeBody
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if !validDate(body.Date) {
		writeError(w, http.StatusBadRequest, "date must be YYYY-MM-DD")
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "at least one ingredient required")
		return
	}
	for _, it := range body.Items {
		if strings.TrimSpace(it.IngredientName) == "" {
			writeError(w, http.StatusBadRequest, "ingredient_name required")
			return
		}
		if it.Quantity <= 0 {
			writeError(w, http.StatusBadRequest, "quantity must be > 0")
			return
		}
	}

	// Synthetic, negative group id: unique per recipe, never collides with a real
	// (positive auto-increment) recipe id, so a custom recipe can't merge with a real
	// recipe group logged on the same day.
	groupID := -time.Now().UnixNano()
	groupName := name

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := h.Q.WithTx(tx)

	out := make([]queries.LogEntry, 0, len(body.Items))
	for _, it := range body.Items {
		entry, err := q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
			UserID:               userID,
			IngredientID:         it.IngredientID, // may be nil for free-form items
			Date:                 body.Date,
			IngredientName:       it.IngredientName,
			IngredientUnit:       it.IngredientUnit,
			CaloriesPerUnit:      it.CaloriesPerUnit,
			ProteinPerUnit:       it.ProteinPerUnit,
			Quantity:             it.Quantity,
			Calories:             it.CaloriesPerUnit * it.Quantity,
			Protein:              it.ProteinPerUnit * it.Quantity,
			SourceRecipeID:       &groupID,
			SourceRecipeName:     &groupName,
			SourceRecipeServings: nil, // no servings multiplier for custom recipes
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, entry)
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
```

> Imports in `log.go`: add `"strings"`. After the cleanup below removes `GetRecentRecipes`,
> also drop the now-unused `"sort"` import from `log.go`. `"time"` stays (used here);
> `"strconv"`/`"net/http"` stay.

### 3. Register the route

`backend/main.go`, in the log section (near line 50-54). Mirrors the existing
`POST /log/recipe` route:

```go
mux.HandleFunc("POST /api/users/{id}/log/custom-recipe", h.LogCustomRecipe)
```

---

## Frontend changes

### 1. Types

`frontend/src/lib/types.ts` — add:

```ts
export interface CustomRecipeItem {
  ingredient_id: number | null
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: number
}

export interface LogCustomRecipePayload {
  name: string
  date: string
  items: CustomRecipeItem[]
}
```

### 2. API client

`frontend/src/lib/api.ts` — add next to `logRecipe` (line ~89), and import the new type:

```ts
logCustomRecipe: (userId: number, payload: LogCustomRecipePayload) =>
  request<LogEntry[]>(`${BASE}/users/${userId}/log/custom-recipe`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }),
```

### 3. New component: `CustomRecipeDrawer.vue`

A bottom-sheet that mirrors `AddRecipeDrawer`'s chrome + viewport handling, but whose body is
the ingredient-picking editor from `RecipeEditor` (minus tags/persistence). Create
`frontend/src/components/CustomRecipeDrawer.vue`:

```vue
<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { Ingredient } from '@/lib/types'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Badge from './ui/Badge.vue'
import { Search, X, Plus, Trash2 } from 'lucide-vue-next'

interface DraftItem {
  ingredient_id: number | null
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: string // string for the numeric input; converted on submit
}

const props = defineProps<{ userId: number; date: string }>()
const emit = defineEmits<{ close: []; added: [] }>()

const name = ref<string>('')
const items = ref<DraftItem[]>([])
const search = ref<string>('')
const searchResults = ref<Ingredient[]>([])
const saving = ref<boolean>(false)
const errMsg = ref<string>('')
let searchTimer: number | undefined

const totalCalories = computed<number>(() =>
  items.value.reduce((sum, it) => {
    const q = Number(it.quantity)
    return Number.isFinite(q) && q > 0 ? sum + q * it.calories_per_unit : sum
  }, 0),
)
const totalProtein = computed<number>(() =>
  items.value.reduce((sum, it) => {
    const q = Number(it.quantity)
    return Number.isFinite(q) && q > 0 ? sum + q * it.protein_per_unit : sum
  }, 0),
)

watch(search, (v) => {
  window.clearTimeout(searchTimer)
  const trimmed = v.trim()
  if (!trimmed) {
    searchResults.value = []
    return
  }
  searchTimer = window.setTimeout(async () => {
    searchResults.value = await api.listIngredients(trimmed)
  }, 200)
})

function addItem(ing: Ingredient): void {
  if (items.value.some((i) => i.ingredient_id === ing.id)) return
  items.value.push({
    ingredient_id: ing.id,
    ingredient_name: ing.name,
    ingredient_unit: ing.unit,
    calories_per_unit: ing.calories_per_unit,
    protein_per_unit: ing.protein_per_unit,
    quantity: '1',
  })
  search.value = ''
  searchResults.value = []
}

function removeItem(idx: number): void {
  items.value.splice(idx, 1)
}

async function logRecipe(): Promise<void> {
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errMsg.value = 'Name required'
    return
  }
  if (items.value.length === 0) {
    errMsg.value = 'Add at least one ingredient'
    return
  }
  const payloadItems = []
  for (const it of items.value) {
    const q = Number(it.quantity)
    if (!Number.isFinite(q) || q <= 0) {
      errMsg.value = `Quantity for ${it.ingredient_name} must be > 0`
      return
    }
    payloadItems.push({
      ingredient_id: it.ingredient_id,
      ingredient_name: it.ingredient_name,
      ingredient_unit: it.ingredient_unit,
      calories_per_unit: it.calories_per_unit,
      protein_per_unit: it.protein_per_unit,
      quantity: q,
    })
  }
  saving.value = true
  errMsg.value = ''
  try {
    await api.logCustomRecipe(props.userId, {
      name: trimmedName,
      date: props.date,
      items: payloadItems,
    })
    emit('added')
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to log recipe'
  } finally {
    saving.value = false
  }
}

// ---- shared drawer chrome: ESC + visual-viewport pinning (copied from AddRecipeDrawer) ----
function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close')
}
const vvHeight = ref<number>(typeof window !== 'undefined' ? window.innerHeight : 0)
const vvTop = ref<number>(0)
function onViewportChange(): void {
  const vv = window.visualViewport
  if (!vv) return
  vvHeight.value = vv.height
  vvTop.value = vv.offsetTop
}
onMounted(() => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', onViewportChange)
    vv.addEventListener('scroll', onViewportChange)
    onViewportChange()
  }
})
onUnmounted(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.removeEventListener('resize', onViewportChange)
    vv.removeEventListener('scroll', onViewportChange)
  }
})
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed left-0 right-0 z-50 flex items-end justify-center"
      :style="{ top: `${vvTop}px`, height: `${vvHeight}px` }"
      @click.self="emit('close')"
    >
      <div class="absolute inset-0 bg-black/50" />
      <div
        class="relative w-full max-w-lg bg-card text-card-foreground border-t border-border rounded-t-2xl p-4 pb-8 flex flex-col gap-3 max-h-[calc(100%-2rem)] overflow-y-auto"
      >
        <div class="mx-auto h-1 w-10 rounded-full bg-border" />

        <div class="flex items-center justify-between">
          <h2 class="font-semibold text-lg">Log a custom recipe</h2>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div>
          <label class="text-xs text-muted-foreground">Recipe name</label>
          <Input v-model="name" placeholder="e.g. Tuesday lunch" />
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
            Ingredients
          </div>
          <div v-if="items.length === 0" class="text-xs text-muted-foreground">
            Search the ingredient library below to add items.
          </div>
          <div
            v-for="(it, idx) in items"
            :key="`${it.ingredient_id ?? 'x'}-${idx}`"
            class="flex items-center gap-2 rounded-md border border-border p-2"
          >
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium truncate">{{ it.ingredient_name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ it.calories_per_unit }} kcal · {{ it.protein_per_unit }} g / 1 {{ it.ingredient_unit }}
              </div>
            </div>
            <Input
              v-model="it.quantity"
              type="number"
              inputmode="decimal"
              min="0"
              step="0.1"
              class="!w-20"
            />
            <span class="text-xs text-muted-foreground w-10">{{ it.ingredient_unit }}</span>
            <Button variant="ghost" size="icon" @click="removeItem(idx)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </div>

        <div class="relative">
          <Input v-model="search" type="search" placeholder="Search ingredient to add…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>
        <div v-if="searchResults.length > 0" class="flex flex-col gap-1 max-h-48 overflow-y-auto">
          <button
            v-for="ing in searchResults"
            :key="ing.id"
            type="button"
            class="text-left p-2 rounded-md border border-border hover:bg-muted transition-colors flex items-center justify-between gap-2"
            @click="addItem(ing)"
          >
            <div class="min-w-0">
              <div class="text-sm font-medium truncate">{{ ing.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ ing.calories_per_unit }} kcal · {{ ing.protein_per_unit }} g / 1 {{ ing.unit }}
              </div>
            </div>
            <Badge variant="outline"><Plus class="h-3 w-3" /></Badge>
          </button>
        </div>

        <div class="rounded-md bg-muted px-3 py-2 flex items-center justify-between">
          <span class="text-xs text-muted-foreground uppercase tracking-wide">Total</span>
          <span class="font-semibold">
            {{ Math.round(totalCalories) }} kcal · {{ formatNumber(totalProtein, 1) }} g protein
          </span>
        </div>

        <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

        <div class="flex justify-end gap-2">
          <Button variant="ghost" size="sm" @click="emit('close')">Cancel</Button>
          <Button size="sm" :disabled="saving" @click="logRecipe">
            {{ saving ? 'Logging…' : 'Log recipe' }}
          </Button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
```

### 4. Wire it into `LogTab.vue`

The **Add** button opens **only** the custom-recipe editor. `LogTab` stops importing and
using `AddRecipeDrawer` entirely.

```ts
// script — replace the AddRecipeDrawer import + showDrawer/onAdded
import CustomRecipeDrawer from '@/components/CustomRecipeDrawer.vue'

const showCustom = ref(false) // replaces showDrawer

function onCustomAdded(): void {
  showCustom.value = false
  void loadLog()
}
```

```vue
<!-- template: the Add button (lines ~163-168) stays a single button, now opening the editor -->
<div class="flex justify-end">
  <Button v-if="sub === 'food'" size="sm" @click="showCustom = true">
    <Plus class="h-2 w-2" />
    Add
  </Button>
</div>
```

```vue
<!-- template: replace the <AddRecipeDrawer> block at the bottom (lines ~279-285) -->
<CustomRecipeDrawer
  v-if="showCustom"
  :user-id="userId"
  :date="today"
  @close="showCustom = false"
  @added="onCustomAdded"
/>
```

Also fix the stale empty-state copy at `LogTab.vue:175` (`tap "Create"` → `tap "Add"`).

---

## Deep cleanup: remove now-redundant functionality

Removing recent-recipe picking from the Log tab orphans the entire "recent recipes" stack and
the drawer's search/recent UI. `AddRecipeDrawer` is **not** deleted, because `TodayTab` still
opens it to log an existing recipe (the `initial-recipe` path, `TodayTab.vue:225-232`); it is
**slimmed** to just that flow.

### Decision — keep `TodayTab`'s "log an existing recipe" flow

The note scopes the change to the **Log → Food Add button**. `TodayTab`'s "Add to log" on a
food-tag recipe card is a separate, still-valid feature, so this plan **keeps** it (and
therefore keeps `logRecipe` / `POST /log/recipe`).

### Frontend — remove

- **`api.recentRecipes`** (`api.ts:86-87`) and the **`RecentItem`** import there (`api.ts:5`).
- **`RecentItem`** interface (`types.ts:36-42`).
- In **`AddRecipeDrawer.vue`**, delete everything that served the recent/search entry point —
  it is now only ever opened with a pre-selected `initialRecipe`:
  - state: `query`, `recent`, `results`, `searchTimer`;
  - the `query` `watch`, `loadRecent()`, `pickRecent()`, `pickRecipe()`;
  - the `loadRecent()` call in `onMounted` and the `RecentItem` import;
  - the search `<Input>`, the "Recent" list, and the "Library" search-results list in the
    template (everything in the `v-else` after the `picked` panel, `AddRecipeDrawer.vue:227-285`).
  - Make `initialRecipe` a **required** prop (rename to `recipe`) and seed `picked` from it
    unconditionally; `clearPicked()` should just `emit('close')` (there is no list to fall
    back to). The drawer becomes a "confirm servings for this recipe" sheet.

### Backend — remove

- **`GetRecentRecipes`** handler, the **`recentItem`** struct, and the `recentWindowDays` /
  `recentItemsCap` consts (`backend/handlers/log.go:12-13,80-124`). Then drop the now-unused
  `"sort"` import from `log.go`.
- The route **`GET /api/users/{id}/recent`** (`backend/main.go:54`).
- The **`GetRecentLoggedRecipes`** query: remove it from `backend/sql/queries/log.sql:6-…`
  and regenerate sqlc (or hand-delete the generated block + `GetRecentLoggedRecipesParams` /
  `GetRecentLoggedRecipesRow` types in `backend/db/queries/log.sql.go:150-217`).

### Keep (shared — do **not** remove)

- **`logRecipe` / `LogRecipePayload` / `LogRecipe` handler / `POST /api/users/{id}/log/recipe`**
  — used by `TodayTab` via the slimmed drawer.
- **`deleteLogRecipeGroup` / `DeleteLogEntriesByRecipe`** — used by `LogTab` to delete groups,
  including custom-recipe groups (this is the handler we relax to allow negative ids).
- **`listRecipes` / `GET /api/recipes`** — still used by `IngredientLibrary.vue:55`.

---

## Edge cases & validation

- **Empty name / no ingredients / non-positive quantity** — validated both client-side
  (inline `errMsg`) and server-side (400). Mirrors `RecipeEditor.save` and `validateRecipeBody`.
- **Duplicate ingredient** — client dedupes by `ingredient_id` (`addItem`), same as
  `RecipeEditor.addIngredient`. Free-form items (null id) are not deduped.
- **Deletion** — works with zero new code: the group row's trash button calls
  `removeRecipeGroup(date, recipeId=groupId, name)` → `deleteLogRecipeGroup` →
  `DELETE /log/recipe?source_recipe_id=<negative>`, now accepted after the `srid == 0` fix.
- **Group-id uniqueness** — `-time.Now().UnixNano()` is unique per request in practice. If
  you want a hard guarantee, allocate the id from a dedicated sequence/table instead; not
  worth it here.

---

## Test / verification plan

Backend (`backend/`):
1. `go build ./...` and `go vet ./...` — confirms the removed `GetRecentRecipes` / `sort`
   import and the new handler all compile cleanly.
2. Manual: `POST /api/users/1/log/custom-recipe` with a name + two items → expect `201` and a
   `LogEntry[]` whose rows share a negative `source_recipe_id` and carry the recipe name.
3. `GET /api/users/1/log` → the new rows appear.
4. `DELETE /api/users/1/log/recipe?date=YYYY-MM-DD&source_recipe_id=<negative>` → `204`,
   rows gone.
5. `GET /api/users/1/recent` → `404` (route removed).

Frontend (`frontend/`):
1. `npx vue-tsc -b` (type check) and `npm run build` — catches any dangling `RecentItem` /
   `recentRecipes` references missed in the cleanup.
2. Manual (`npm run dev`): Log → Food → **Add** → enter name, add a couple of library
   ingredients with quantities, confirm the live total, **Log recipe** → drawer closes, the
   recipe shows as a grouped block for today with a "Recipe" badge and correct totals.
3. Delete the group via its trash icon → it disappears and the day total updates.
4. **Regression:** Today tab → open a food tag → "Add to log" on a recipe → the slimmed
   drawer still logs it with the chosen servings.

---

## Out of scope / future

- **Free-form ingredients** (type a name + calories without a library ingredient): the
  backend endpoint already supports `ingredient_id: null`; only the drawer UI would need an
  "add manual item" affordance.
- **"Save as recipe"** convenience action after logging a custom recipe (calls
  `api.createRecipe` with the same items).
- **Dedicated `ad_hoc_group_id` column** if ad-hoc recipes become first-class and we want to
  drop the negative-id convention.
