# Editable Food Items

## Goal

Today, foods can be created and deleted but not edited. The UI exposes a pencil button on recipe rows but not on food rows. Add edit-in-place for foods, matching the recipe UX:

- Pencil icon on each food row in the Library → opens an editor dialog pre-filled with the current values.
- **Only `calories_per_unit` is editable.** `name` and `unit` are displayed read-only. This keeps the editor scope tight and sidesteps every unit-related historical inconsistency.
- The same dialog also handles the "New food" path (parallel to `RecipeEditor.vue` for recipes). In create mode all three fields are editable; in edit mode only the calories field is.
- Backend: new `PUT /api/foods/{id}` endpoint that accepts only `calories_per_unit`.

### Why this is safe

- **Log entries are kept in sync.** When `calories_per_unit` changes, the same transaction also rewrites every `log_entries` row that still references this food (`food_id = ?`): both `calories_per_unit` and the derived `calories = new_cpu × quantity`. Historical totals therefore reflect the corrected calorie figure. Log entries whose `food_id` is `NULL` (i.e. the food was previously deleted) are left untouched, preserving the data we have.
- **Recipes are live-joined.** `ListRecipes` and `GetRecipeIngredients` read calories straight from the `foods` table, so a food edit immediately changes the calorie total of every recipe that uses it. This is the intended behaviour — fixing "mango = 0.6 kcal/g" to "mango = 0.65 kcal/g" should update the shake's total.
- **No unit edits** means recipe ingredient quantities (stored in the food's unit) and log-entry `food_unit` snapshots never go out of sync.

---

## Phase 1 — Backend

### `backend/sql/queries/foods.sql`

Add two queries: one to update the food's calories, one to restamp every dependent log entry.

```sql
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
```

`food_id` is nullable on `log_entries` (set to NULL when a food was previously deleted), so the `WHERE food_id = ?` predicate naturally excludes those rows — exactly what we want.

Run:

```bash
cd backend && sqlc generate
```

### `backend/handlers/foods.go`

Add `UpdateFood`. The handler is intentionally narrow — only `calories_per_unit` in the body — and runs a transaction so the food row and its log-entry snapshots either both update or neither does.

```go
type updateFoodBody struct {
    CaloriesPerUnit float64 `json:"calories_per_unit"`
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

    tx, err := h.DB.BeginTx(r.Context(), nil)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    defer func() { _ = tx.Rollback() }()
    q := h.Q.WithTx(tx)

    food, err := q.UpdateFoodCalories(r.Context(), queries.UpdateFoodCaloriesParams{
        ID:              id,
        CaloriesPerUnit: body.CaloriesPerUnit,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    if err := q.RestampLogEntriesForFood(r.Context(), queries.RestampLogEntriesForFoodParams{
        CaloriesPerUnit: body.CaloriesPerUnit,
        FoodID:          &id,
    }); err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    if err := tx.Commit(); err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    writeJSON(w, http.StatusOK, food)
}
```

`errors` and `database/sql` need to be in the imports. The transaction pattern mirrors `LogRecipe` in `handlers/recipes.go` — `Handler` already holds `*sql.DB`.

Note on `RestampLogEntriesForFoodParams.FoodID`: sqlc generates it as `*int64` because `log_entries.food_id` is nullable; pass the address of the local `id`.

### Route in `main.go`

```go
mux.HandleFunc("GET /api/foods", h.ListFoods)
mux.HandleFunc("POST /api/foods", h.CreateFood)
mux.HandleFunc("PUT /api/foods/{id}", h.UpdateFood) // new
mux.HandleFunc("DELETE /api/foods/{id}", h.DeleteFood)
```

---

## Phase 2 — Frontend types + API client

### `frontend/src/lib/types.ts`

Only `calories_per_unit` is editable, so the payload mirrors that:

```ts
export interface UpdateFoodPayload {
  calories_per_unit: number
}
```

Keep `CreateFoodPayload` as-is.

### `frontend/src/lib/api.ts`

```ts
updateFood: (id: number, payload: UpdateFoodPayload) =>
  request<Food>(`${BASE}/foods/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  }),
```

---

## Phase 3 — Frontend: extract `FoodEditor.vue`

Pull the inline "New food" `Dialog` out of `FoodLibrary.vue` into its own component, mirroring `RecipeEditor.vue` so the create / edit modes share one implementation.

`frontend/src/components/FoodEditor.vue`:

```vue
<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { api } from '@/lib/api'
import type { Food } from '@/lib/types'
import Dialog from '@/components/ui/Dialog.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'

const UNITS = ['g', 'ml', 'oz', 'piece', 'tbsp', 'cup', 'serving']

const props = defineProps<{ open: boolean; food: Food | null }>()
const emit = defineEmits<{
  'update:open': [v: boolean]
  saved: []
}>()

const isEdit = computed<boolean>(() => props.food != null)

const name = ref<string>('')
const unit = ref<string>('g')
const calories = ref<string>('')
const saving = ref<boolean>(false)
const errMsg = ref<string>('')
const aiHint = ref<string>('')
const aiLoading = ref<boolean>(false)
const aiError = ref<string>('')

function reset(): void {
  name.value = props.food?.name ?? ''
  unit.value = props.food?.unit ?? 'g'
  calories.value = props.food != null ? String(props.food.calories_per_unit) : ''
  errMsg.value = ''
  aiHint.value = ''
  aiError.value = ''
  aiLoading.value = false
}

watch(
  () => props.open,
  (open) => {
    if (open) reset()
  },
)

async function fetchCalorieHint(): Promise<void> {
  const trimmed = name.value.trim()
  if (!trimmed) return
  aiHint.value = ''
  aiError.value = ''
  aiLoading.value = true
  try {
    const res = await api.calorieHint(trimmed)
    aiHint.value = res.hint
  } catch {
    aiError.value = 'Could not fetch AI hint.'
  } finally {
    aiLoading.value = false
  }
}

async function save(): Promise<void> {
  const cal = Number(calories.value)
  if (!Number.isFinite(cal) || cal < 0) {
    errMsg.value = 'Enter a non-negative calorie value'
    return
  }
  if (!isEdit.value && !name.value.trim()) {
    errMsg.value = 'Name required'
    return
  }
  saving.value = true
  errMsg.value = ''
  try {
    if (props.food != null) {
      await api.updateFood(props.food.id, { calories_per_unit: cal })
    } else {
      await api.createFood({
        name: name.value.trim(),
        unit: unit.value,
        calories_per_unit: cal,
      })
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to save food'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    :open="open"
    :title="food ? 'Edit food' : 'New food'"
    @update:open="(v) => emit('update:open', v)"
  >
    <div class="flex flex-col gap-3">
      <template v-if="isEdit">
        <div>
          <div class="text-xs text-muted-foreground">Food</div>
          <div class="font-medium">{{ name }}</div>
        </div>
      </template>
      <template v-else>
        <div class="flex gap-2">
          <Input v-model="name" placeholder="Food name" class="flex-1" />
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="aiLoading || !name.trim()"
            @click="fetchCalorieHint"
            title="Ask AI for calorie info"
          >
            <span v-if="aiLoading" class="animate-pulse">…</span>
            <span v-else>✨ AI</span>
          </Button>
        </div>
        <p v-if="aiHint" class="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground leading-snug">
          {{ aiHint }}
        </p>
        <p v-if="aiError" class="text-xs text-destructive">{{ aiError }}</p>
      </template>

      <div class="flex gap-2">
        <Input
          v-model="calories"
          type="number"
          inputmode="decimal"
          placeholder="Calories"
          min="0"
          step="0.1"
        />
        <template v-if="isEdit">
          <div class="h-9 rounded-md border border-input bg-muted px-3 text-sm grid place-items-center text-muted-foreground">
            {{ unit }}
          </div>
        </template>
        <template v-else>
          <select
            v-model="unit"
            class="h-9 rounded-md border border-input bg-background px-2 text-sm"
          >
            <option v-for="u in UNITS" :key="u" :value="u">{{ u }}</option>
          </select>
        </template>
      </div>
      <p class="text-xs text-muted-foreground">Calories per 1 {{ unit }}</p>
      <p v-if="isEdit" class="text-xs text-muted-foreground">
        Changing this updates all past log entries for {{ name }}.
      </p>
      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="emit('update:open', false)">Cancel</Button>
        <Button :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
        </Button>
      </div>
    </div>
  </Dialog>
</template>
```

The component:
- Receives `food: Food | null` — `null` means create, a `Food` object means edit.
- In **edit** mode `name` and `unit` are rendered read-only; only the calorie input is editable. A short explanation tells the user that the change propagates to past log entries.
- In **create** mode all three fields and the `✨ AI` hint are available exactly like before.
- Re-syncs its local refs every time the dialog re-opens via `watch(() => props.open)`.
- Emits `saved` so the parent can refetch.

---

## Phase 4 — `FoodLibrary.vue` integration

Replace the inline "New food" dialog with `<FoodEditor>`, drop the now-unused refs/handlers, and add a pencil button to each food row.

```diff
- import Dialog from '@/components/ui/Dialog.vue'
+ import FoodEditor from '@/components/FoodEditor.vue'
- import { ChevronLeft, Search, Trash2, Plus, Pencil } from 'lucide-vue-next'
+ import { ChevronLeft, Search, Trash2, Plus, Pencil } from 'lucide-vue-next'  // already imported
```

Remove the food-create state and helpers (`newFood`, `savingFood`, `errMsg`, `aiHint`, `aiLoading`, `aiError`, `addFood`, `fetchCalorieHint`, and the `watch(showAddFood)` block) — they now live inside `FoodEditor`.

Replace with editor-driver state matching the recipe pattern:

```ts
const showFoodEditor = ref<boolean>(false)
const editingFood = ref<Food | null>(null)

function openNewFood(): void {
  editingFood.value = null
  showFoodEditor.value = true
}

function openEditFood(food: Food): void {
  editingFood.value = food
  showFoodEditor.value = true
}

function onFoodSaved(): void {
  void loadFoods()
}
```

In the template, the food row gets a pencil button next to the trash, and the "Add food" button calls `openNewFood`:

```vue
<Card v-for="f in foods" :key="f.id">
  <div class="p-3 flex items-center gap-3">
    <div class="flex-1 min-w-0">
      <div class="font-medium truncate">{{ f.name }}</div>
      <div class="text-xs text-muted-foreground">
        {{ f.calories_per_unit }} kcal / 1 {{ f.unit }}
      </div>
    </div>
    <Badge variant="secondary">{{ f.unit }}</Badge>
    <Button variant="ghost" size="icon" @click="openEditFood(f)">
      <Pencil class="h-4 w-4" />
    </Button>
    <Button variant="ghost" size="icon" @click="deleteFood(f.id)">
      <Trash2 class="h-4 w-4" />
    </Button>
  </div>
</Card>

<Button class="mt-2" @click="openNewFood">
  <Plus class="h-4 w-4" />
  Add food
</Button>
```

Replace the previous `<Dialog v-model:open="showAddFood">…</Dialog>` block with:

```vue
<FoodEditor
  v-model:open="showFoodEditor"
  :food="editingFood"
  @saved="onFoodSaved"
/>
```

---

## Phase 5 — Testing checklist

- [ ] `PUT /api/foods/{id}` with `{calories_per_unit: 0.65}` → 200, returns updated row with new value.
- [ ] `PUT /api/foods/{id}` with `calories_per_unit < 0` → 400.
- [ ] `PUT /api/foods/{nonexistent_id}` → 404.
- [ ] Edit a food's calories: every existing `log_entries` row with `food_id = thatID` now has the new `calories_per_unit` and a recomputed `calories = new_cpu × quantity`.
- [ ] Log entries with `food_id IS NULL` (food was previously deleted) are **not** touched by the restamp.
- [ ] Edit a food's calories: any recipe using that food immediately shows the new `total_calories` in `GET /api/recipes`.
- [ ] Edit a food's calories: the user's "today" donut on Home reflects the recomputed totals.
- [ ] FoodLibrary pencil opens the editor pre-filled with the current calories; `name` and `unit` are shown but not editable.
- [ ] FoodLibrary "Add food" still works through the same `FoodEditor` (all three fields editable when `food` prop is `null`).
- [ ] `vue-tsc -b --force` clean.
- [ ] `go build ./...` clean.

---

## Phase 6 — Rollout order

1. **Backend** — `UpdateFoodCalories` + `RestampLogEntriesForFood` sqlc queries, transactional `UpdateFood` handler, route.
2. **Frontend types + API client** — `UpdateFoodPayload`, `api.updateFood`.
3. **Extract `FoodEditor.vue`** (create-mode parity first; verify "Add food" still works).
4. **Wire edit mode** — pencil button on food rows, `editingFood` state, removed inline dialog, name/unit rendered read-only.
5. **Manual smoke test** — create a food, log an entry, edit the food's calories, confirm the historical entry's `calories_per_unit` and `calories` were rewritten, and a recipe using it shows the new total.
6. **Update `research.md`** — add `PUT /api/foods/{id}` to the endpoint table; document that food-calorie edits are propagated to existing `log_entries` (no longer a pure snapshot for cpu), while name/unit remain immutable post-create.

---

## Open considerations

- **Editing name or unit later.** Out of scope by design. If ever needed, the same handler can accept additional fields and a parallel `RestampLogEntriesForFood` query can also rewrite `food_name`/`food_unit` snapshots — but unit changes still leave recipe quantities semantically ambiguous (200g → 200 piece) and would need a separate decision.
- **Restamp on a deleted-and-recreated food.** If a food was previously deleted (its log entries' `food_id` is now `NULL`) and a new food with the same name is created and later edited, those orphaned log entries are *not* updated. That's correct — they were intentionally severed when the original food was deleted.
- **Optimistic UI.** The current pattern refetches the food list after save. For a household-scale app this is fine; if latency matters, the editor could emit the updated `Food` object and the parent could replace it in-place.
- **`PATCH` vs `PUT`.** The body has one field, so the distinction is academic. Sticking with `PUT` to match the existing `PUT /api/users/{id}` style in the codebase.
- **Edit history.** Not tracked. If audit becomes important later, add a `food_revisions` table written in the same transaction.
