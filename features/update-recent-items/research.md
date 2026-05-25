# Research: "Get Recent Foods" feature

A focused deep-dive into how the "recent foods" list works end-to-end in this
codebase — the SQL, the handler, the wire format, the Vue consumer, and the
specific quirks of each layer.

---

## 1. What the feature is for

The Add-to-log drawer (`AddFoodDrawer.vue`) is the user's main mechanism for
logging a meal. Searching the whole library every time is slow when most days
people eat the same handful of foods. The "Recent" section is the fast path:
when the drawer opens and the search box is empty, it shows a per-user list of
recently logged foods, each row pre-populated with the quantity from the last
time that food was logged. One tap selects the food and primes the confirm
form so the user only has to adjust the number if needed.

The list is intentionally **per-user**, **food-only (no recipes)**, and
**snapshot-based** — it reflects what was actually logged historically, not
the current state of the food library.

---

## 2. End-to-end path

```
AddFoodDrawer.onMounted()
  → api.recentFoods(userId)                          (frontend/lib/api.ts)
  → GET /api/users/{id}/recent-foods                 (registered in main.go)
  → Handler.GetRecentFoods                           (handlers/log.go)
  → queries.Queries.GetRecentLoggedFoods             (sqlc-generated)
  → SQL: GetRecentLoggedFoods                        (sql/queries/log.sql)
  → []GetRecentLoggedFoodsRow                        ← rows back
  → []RecentFood                                     ← JSON to client
  → recent.value                                     ← Vue ref
  → renders "Recent" list, click → pickRecent(item)  ← prefilled form
```

---

## 3. Database layer

### The query (`backend/sql/queries/log.sql`)

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

This is a textbook "latest row per group" pattern, written without window
functions (SQLite has them, but this style stays compatible with older
SQLite builds and is simple to reason about):

1. **Inner subquery** groups `log_entries` by `food_name` and picks the
   highest `id` per group, scoped to one user and excluding recipe-derived
   rows.
2. **Outer query** joins those `max_id`s back to the full `log_entries` rows
   to pull the full snapshot of that row's columns.
3. **`ORDER BY le.id DESC` + `LIMIT 20`** caps the result and orders newest
   first.

### What it returns (sqlc-generated, `db/queries/log.sql.go`)

```go
type GetRecentLoggedFoodsRow struct {
    FoodName        string  `json:"food_name"`
    FoodUnit        string  `json:"food_unit"`
    CaloriesPerUnit float64 `json:"calories_per_unit"`
    ProteinPerUnit  float64 `json:"protein_per_unit"`
    FoodID          *int64  `json:"food_id"`
    LastQuantity    float64 `json:"last_quantity"`
}
```

Note: `FoodID` is a pointer because `log_entries.food_id` is nullable —
deleting a food in the library sets `food_id` to NULL via the FK's
`ON DELETE SET NULL` rule, but the historical log row (and therefore the
recent-foods entry) survives.

### Why these columns are read off `log_entries`, not `foods`

The query reads `food_name`, `food_unit`, `calories_per_unit`,
`protein_per_unit` directly off the log row — not by joining to `foods`.
The schema deliberately snapshots those values into `log_entries` at log
time. Consequences:

- If the user **edits the food** in the library afterwards (e.g. corrects
  "Banana, 90 kcal" to "Banana, 105 kcal"), recent-foods still shows the
  per-unit calories that were logged the **last time** they ate a banana.
- If the food is **deleted** from the library, recent-foods still shows it
  because the snapshot persists.
- The food_id in the row is the *original* food_id (or null), used by
  `pickRecent()` only to populate the new log entry's `food_id` link if it
  still exists.

### Filters applied

- `user_id = ?1` — scoped per user. Two members of the same household don't
  pollute each other's recent list.
- `source_recipe_id IS NULL` — entries created by expanding a recipe
  (mango/milk/sugar from "Mango Shake") are excluded. The Recent picker
  is intentionally a list of standalone foods; if a user wants to log a
  recipe again, the search box handles that flow.

### Indexes touched

`idx_log_user_date ON log_entries(user_id, date)` exists. There is **no**
index on `(user_id, food_name)`, so the inner GROUP BY does a filtered
scan + hash aggregation. For household-scale data (tens of thousands of
log rows) this is fine.

---

## 4. Backend handler

### Route (`backend/main.go:55`)

```go
mux.HandleFunc("GET /api/users/{id}/recent-foods", h.GetRecentFoods)
```

Standard Go 1.22 `ServeMux` pattern: method-prefixed path with one
parameter.

### Handler (`backend/handlers/log.go:152`)

```go
func (h *Handler) GetRecentFoods(w http.ResponseWriter, r *http.Request) {
    userID, err := parseID(r, "id")
    if err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    rows, err := h.Q.GetRecentLoggedFoods(r.Context(), userID)
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error())
        return
    }
    if rows == nil {
        rows = []queries.GetRecentLoggedFoodsRow{}
    }
    writeJSON(w, http.StatusOK, rows)
}
```

A few things worth calling out:

- **No date or limit query params.** The cap of 20 lives in the SQL.
- **No auth check.** Like the rest of the app, `parseID` only checks that
  the path id is a positive integer — anyone hitting the endpoint with a
  user id gets that user's recent foods. This matches the project's "no
  auth, trusted LAN" design stance.
- **Empty-slice normalisation.** `sqlc` returns a nil slice when there
  are zero rows; the handler swaps it for an empty slice so the JSON
  response is `[]` rather than `null`. The frontend assumes an array
  and would otherwise crash on `.map`/`.length`.
- **No transaction needed** — the query is a single read.

### Wire shape

```json
[
  {
    "food_name":         "Banana",
    "food_unit":         "piece",
    "calories_per_unit": 105,
    "protein_per_unit":  1.3,
    "food_id":           42,
    "last_quantity":     2
  },
  ...
]
```

---

## 5. Frontend layer

### Type (`frontend/src/lib/types.ts:35`)

```ts
export interface RecentFood {
  food_name: string
  food_unit: string
  calories_per_unit: number
  protein_per_unit: number
  food_id: number | null
  last_quantity: number
}
```

`food_id` is `number | null` — matches the Go `*int64`.

### API client (`frontend/src/lib/api.ts:81`)

```ts
recentFoods: (userId: number) =>
  request<RecentFood[]>(`${BASE}/users/${userId}/recent-foods`),
```

Trivial wrapper; throws an `Error` carrying the backend's `error` field on
non-2xx responses.

### Consumer: `AddFoodDrawer.vue`

The drawer is the only consumer of this endpoint. Lifecycle:

1. **Mount.** `onMounted` calls `loadRecent()`, which awaits
   `api.recentFoods(props.userId)` and stores the result in `recent`.
2. **Render.** When the search box is empty (`!query.trim()`), the
   "Recent" section iterates `recent` and renders each as a button row
   showing:
   ```
   Banana
   2 piece · 210 kcal last time          [210]
   ```
   The right-hand badge and the inline kcal are both computed client-side
   as `Math.round(item.last_quantity * item.calories_per_unit)` — using
   the snapshotted per-unit value, exactly what was actually logged.
3. **Pick.** Clicking a row calls `pickRecent(item)`, which seeds the
   `picked` ref with a `PickedFood`:
   ```ts
   picked.value = {
     kind: 'food',
     food_id: item.food_id,                  // may be null
     food_name: item.food_name,
     food_unit: item.food_unit,
     calories_per_unit: item.calories_per_unit,
     protein_per_unit: item.protein_per_unit,
     quantity: String(item.last_quantity),   // prefilled
   }
   ```
   The drawer's UI then switches to the "selected food" view where the
   user can adjust quantity or hit "Add to log" straight away.
4. **Confirm.** Hitting "Add to log" POSTs `/api/users/{id}/log` with the
   snapshot values, including `food_id` (or null) — the new entry inherits
   the last logged calories/protein per unit even if the food has since
   been edited in the library.

### When the list refreshes

It doesn't, while the drawer is open. `loadRecent()` is only called from
`onMounted`. After a user adds an entry via this drawer, the drawer emits
`added`, which the parent (`UserPage.vue`) handles by:

```ts
async function onFoodAdded() {
  showDrawer.value = false
  await loadLog()
}
```

— it closes the drawer and reloads the log for the day. The next time the
user opens the drawer, `onMounted` fires again and the recent list is
re-fetched, so the just-added food will surface (and may now be at the top
if its `id` is highest). Within a single drawer session the recent list is
a snapshot.

---

## 6. Specifics & quirks worth knowing

- **"Recency" is by row `id`, not by `date`.** The inner subquery picks
  `MAX(id)` per `food_name`, and the outer `ORDER BY le.id DESC` sorts on
  insert order. If a user back-dates an entry (logs yesterday's lunch this
  morning), it still ranks as the *most recent log* of that food because
  its `id` is newer, even though its `date` is older. This is consistent
  with "recent activity" semantics, not "most recently consumed."

- **Grouping key is `food_name` alone.** Not `(food_name, food_unit)` or
  `(food_name, food_unit, calories_per_unit)`. If two log rows share a
  name but differ in unit or per-unit calories (possible if the food was
  edited between logs, or a typo created two foods with the same name),
  only the latest one wins. The earlier variant disappears from the list
  even though it has a distinct `food_id`.

- **The Vue `v-for` `:key` is `item.food_name`.** This mirrors the
  SQL grouping. If two recent entries somehow shared a name across users
  it wouldn't matter (the query is user-scoped), but it does mean the key
  is a string and the assumption "name is unique within the recent list"
  is load-bearing.

- **Recipe-derived rows are excluded.** Adding "Mango Shake" inserts log
  rows for mango/milk/sugar but these never appear in Recent because their
  `source_recipe_id` is non-null. Without this filter, logging a single
  recipe would flood Recent with its ingredients and shove out genuinely
  habitual foods.

- **`food_id` survives food deletion as `NULL`.** Recent rows can have
  `food_id: null`. When the user picks such a row, the subsequent
  `addLog` POST sends `food_id: null` — the backend happily creates the
  new entry without a foreign-key link. Calorie history is preserved at
  the cost of a dangling food name; this is by design.

- **`calories_per_unit` and `protein_per_unit` are snapshots from the
  last log, not from `foods`.** If a user fixes a wrong calorie value in
  the library, Recent will keep showing the old per-unit number until
  they log the food again. Whether this is the right UX is debatable —
  one could argue Recent should re-join `foods` to surface current
  values — but the current design favours "show me what I ate last time."

- **Limit is hard-coded at 20.** No pagination, no client param. With 20
  distinct foods this likely covers a household for weeks; with a
  power-user logging hundreds of distinct items it would clip silently.
  Tuning would require either a query param on the endpoint or making
  the limit configurable.

- **Performance.** No dedicated index supports the inner `GROUP BY
  food_name`. SQLite uses `idx_log_user_date` to narrow by user, then
  scans + aggregates. Acceptable up to tens of thousands of rows. If
  ever pushed to millions, a covering index like
  `(user_id, source_recipe_id, food_name, id DESC)` would let SQLite
  satisfy the inner query with an index scan.

- **The handler returns an empty array, never 404.** Even if the user
  has never logged a food (or doesn't exist at all — the handler doesn't
  verify the user is real, only that the id is a positive integer), the
  response is `[]`. The frontend's empty-state copy
  ("No recent foods — search above to add one.") covers this.

- **No caching.** The drawer hits the endpoint on every open. For a
  drawer that's typically opened once or twice per meal, this is fine
  and keeps the list authoritative.

- **Snapshot-driven correctness story.** The whole feature inherits
  correctness from a design choice elsewhere: every log row stores
  food_name/food_unit/calories_per_unit/protein_per_unit at write time.
  That means recent-foods doesn't have to chase the current state of
  `foods` — it just reads whatever the log says. Combined with the
  recipe-source exclusion, this gives a clean, deterministic list with
  one tiny query and no client-side post-processing.
