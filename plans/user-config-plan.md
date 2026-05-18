# Move `target_calories` from `daily_metrics` to `users`

## Goal

Today, `target_calories` is per-(user, date) on `daily_metrics`. In practice nobody changes their daily target — it's a profile setting. Move it to the `users` table so:

- **Creating a user** requires `{ name, target_calories }`.
- **Existing users** can update it through the same "Target calories" input on `UserPage` — but the request now hits `PUT /api/users/{id}` instead of `PUT /api/users/{id}/metrics`.
- `daily_metrics` drops the column entirely; nothing in the app references it after migration.
- `TodaySummary` and `ProgressCharts` source the target from `users.target_calories`.
- **Migration is non-preserving on purpose**: on legacy DBs every existing user is initialised to the default `2000`. We do **not** backfill from `daily_metrics`. Users can adjust their target in the UI immediately afterwards. This keeps the migration to two trivial `ALTER TABLE` calls with no conditional logic.

### Side effects

- The progress chart's target reference line becomes a single horizontal line (current `users.target_calories`) instead of a step function — acceptable, and arguably more correct since the target is conceptually a setting, not a daily value.
- Changing the target retroactively re-renders all historical charts with the new line. This matches user intent ("show me how I did against my current goal").

---

## Phase 1 — Schema + migration

### Update `backend/sql/schema.sql`

Add the column to `users`. Remove it from `daily_metrics`.

```sql
CREATE TABLE IF NOT EXISTS users (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  name            TEXT    NOT NULL,
  avatar          TEXT    NOT NULL,
  target_calories INTEGER NOT NULL DEFAULT 2000,
  created_at      TEXT    NOT NULL DEFAULT (date('now'))
);

CREATE TABLE IF NOT EXISTS daily_metrics (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date    TEXT    NOT NULL,
  weight  REAL,
  steps   INTEGER,
  UNIQUE(user_id, date)
);
```

The `DEFAULT 2000` exists so the `ADD COLUMN` migration on legacy DBs is well-defined. New users always come through the API which **requires** `target_calories`, so the default is only a fallback for backfill.

Remember to keep `backend/db/schema.sql` (the embedded copy) in sync.

### Migration helpers in `backend/db/db.go`

`ensureColumn` stays exactly as it is today (returns just `error`) — because we're relying on SQLite's behaviour that `ALTER TABLE … ADD COLUMN … NOT NULL DEFAULT 2000` populates existing rows with the default value. No conditional backfill code is needed.

Add a sibling `dropColumnIfExists` since SQLite ≥ 3.35 supports `ALTER TABLE … DROP COLUMN` and `modernc.org/sqlite` is current enough.

```go
func dropColumnIfExists(conn *sql.DB, table, column string) error {
    var existing string
    err := conn.QueryRow(
        "SELECT name FROM sqlite_master WHERE type='table' AND name=?",
        table,
    ).Scan(&existing)
    if err == sql.ErrNoRows {
        return nil
    }
    if err != nil {
        return err
    }
    rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
    if err != nil {
        return err
    }
    defer rows.Close()
    found := false
    for rows.Next() {
        var cid int
        var name, ctype string
        var notnull, pk int
        var dflt sql.NullString
        if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
            return err
        }
        if name == column {
            found = true
        }
    }
    if err := rows.Err(); err != nil {
        return err
    }
    if !found {
        return nil
    }
    _, err = conn.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column))
    return err
}
```

### Wire migrations in `Init`

Two operations, no ordering subtlety: add the column on `users` (SQLite fills existing rows with `2000`), then drop the column from `daily_metrics`.

```go
// pre-schema migrations (these need to run before any CREATE INDEX that references
// the new column, and before the daily_metrics column is dropped)
if err := ensureColumn(conn, "log_entries", "source_recipe_id", "INTEGER"); err != nil { ... }
if err := ensureColumn(conn, "log_entries", "source_recipe_name", "TEXT"); err != nil { ... }

if err := ensureColumn(conn, "users", "target_calories", "INTEGER NOT NULL DEFAULT 2000"); err != nil {
    return nil, fmt.Errorf("migrate users.target_calories: %w", err)
}
if err := dropColumnIfExists(conn, "daily_metrics", "target_calories"); err != nil {
    return nil, fmt.Errorf("drop daily_metrics.target_calories: %w", err)
}

// then the schema CREATE TABLEs / CREATE INDEXes
for _, stmt := range splitStatements(schemaSQL) { ... }
```

The existing `log_entries` `ensureColumn` calls stay exactly as they are — no signature change.

---

## Phase 2 — SQL queries + sqlc

### `backend/sql/queries/users.sql`

```sql
-- name: ListUsers :many
SELECT * FROM users ORDER BY id;

-- name: GetUser :one
SELECT * FROM users WHERE id = ?;

-- name: CreateUser :one
INSERT INTO users (name, avatar, target_calories)
VALUES (?, ?, ?)
RETURNING *;

-- name: UpdateUser :one
UPDATE users
SET name            = ?,
    target_calories = ?
WHERE id = ?
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = ?;
```

### `backend/sql/queries/metrics.sql`

Drop `target_calories` from `UpsertMetrics`, `GetMetricsRange`, `GetTodaySummary`.

```sql
-- name: UpsertMetrics :one
INSERT INTO daily_metrics (user_id, date, weight, steps)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id, date) DO UPDATE SET
  weight = excluded.weight,
  steps  = excluded.steps
RETURNING *;

-- name: GetMetricsRange :many
SELECT id, user_id, date, weight, steps
FROM daily_metrics
WHERE user_id = sqlc.arg(user_id)
  AND date >= sqlc.arg(from_date)
  AND date <= sqlc.arg(to_date)
ORDER BY date;

-- name: GetTodaySummary :one
SELECT
  CAST(COALESCE((
    SELECT SUM(calories) FROM log_entries
    WHERE user_id = u.id AND date = date('now')
  ), 0) AS REAL) AS consumed,
  u.target_calories AS target
FROM users u
WHERE u.id = ?;
```

Then:

```bash
cd backend && sqlc generate
```

---

## Phase 3 — Backend handlers

### `handlers/users.go`

```go
type createUserBody struct {
    Name           string `json:"name"`
    Avatar         string `json:"avatar"`
    TargetCalories int64  `json:"target_calories"`
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    var body createUserBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    name := strings.TrimSpace(body.Name)
    if name == "" {
        writeError(w, http.StatusBadRequest, "name required"); return
    }
    if body.TargetCalories <= 0 {
        writeError(w, http.StatusBadRequest, "target_calories must be > 0"); return
    }
    avatar := strings.TrimSpace(body.Avatar)
    if avatar == "" {
        avatar = strings.ToUpper(name)
        if len(avatar) > 2 { avatar = avatar[:2] }
    }
    u, err := h.Q.CreateUser(r.Context(), queries.CreateUserParams{
        Name: name, Avatar: avatar, TargetCalories: body.TargetCalories,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    writeJSON(w, http.StatusCreated, u)
}

type updateUserBody struct {
    Name           *string `json:"name"`
    TargetCalories *int64  `json:"target_calories"`
}

func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
    id, err := parseID(r, "id")
    if err != nil { writeError(w, http.StatusBadRequest, err.Error()); return }

    current, err := h.Q.GetUser(r.Context(), id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            writeError(w, http.StatusNotFound, "user not found"); return
        }
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }

    var body updateUserBody
    if err := readJSON(r, &body); err != nil {
        writeError(w, http.StatusBadRequest, err.Error()); return
    }
    name := current.Name
    target := current.TargetCalories
    if body.Name != nil {
        trimmed := strings.TrimSpace(*body.Name)
        if trimmed == "" {
            writeError(w, http.StatusBadRequest, "name must be non-empty"); return
        }
        name = trimmed
    }
    if body.TargetCalories != nil {
        if *body.TargetCalories <= 0 {
            writeError(w, http.StatusBadRequest, "target_calories must be > 0"); return
        }
        target = *body.TargetCalories
    }
    u, err := h.Q.UpdateUser(r.Context(), queries.UpdateUserParams{
        ID: id, Name: name, TargetCalories: target,
    })
    if err != nil {
        writeError(w, http.StatusInternalServerError, err.Error()); return
    }
    writeJSON(w, http.StatusOK, u)
}
```

### `handlers/metrics.go`

Drop `target_calories` from `metricsResponse` and `upsertMetricsBody`. `GetTodaySummary` no longer needs the LEFT JOIN — the sqlc query above selects directly from `users`.

```go
type metricsResponse struct {
    ID               int64    `json:"id"`
    UserID           int64    `json:"user_id"`
    Date             string   `json:"date"`
    Weight           *float64 `json:"weight"`
    Steps            *int64   `json:"steps"`
    CaloriesConsumed float64  `json:"calories_consumed"`
}

type upsertMetricsBody struct {
    Date   string   `json:"date"`
    Weight *float64 `json:"weight"`
    Steps  *int64   `json:"steps"`
}
```

Remove the `TargetCalories` validation and the parameter from `UpsertMetricsParams`.

### Routes in `main.go`

Add the new update endpoint:

```go
mux.HandleFunc("GET    /api/users",        h.ListUsers)
mux.HandleFunc("POST   /api/users",        h.CreateUser)
mux.HandleFunc("PUT    /api/users/{id}",   h.UpdateUser) // new
mux.HandleFunc("DELETE /api/users/{id}",   h.DeleteUser)
mux.HandleFunc("GET    /api/users/{id}/today", h.GetTodaySummary)
```

### Seed (`cmd/seed/main.go`)

Pass `TargetCalories` when creating users; stop setting it inside `UpsertMetrics`.

```go
users := []queries.CreateUserParams{
    {Name: "Mohit", Avatar: "MO", TargetCalories: 2200},
    {Name: "Sara",  Avatar: "SR", TargetCalories: 1800},
}
// ...
if _, err := q.UpsertMetrics(ctx, queries.UpsertMetricsParams{
    UserID: u.ID, Date: date, Weight: &weight, Steps: &steps,
}); err != nil { ... }
```

---

## Phase 4 — Frontend types + API client

### `frontend/src/lib/types.ts`

```ts
export interface User {
  id: number
  name: string
  avatar: string
  target_calories: number
  created_at: string
}

export interface DailyMetric {
  id: number
  user_id: number
  date: string
  weight: number | null
  steps: number | null
  calories_consumed: number
}

export interface MetricsUpdate {
  date: string
  weight: number | null
  steps: number | null
}

export interface CreateUserPayload {
  name: string
  target_calories: number
}

export interface UpdateUserPayload {
  name?: string
  target_calories?: number
}
```

### `frontend/src/lib/api.ts`

```ts
createUser: (payload: CreateUserPayload) =>
  request<User>(`${BASE}/users`, {
    method: 'POST',
    body: JSON.stringify(payload),
  }),

updateUser: (id: number, payload: UpdateUserPayload) =>
  request<User>(`${BASE}/users/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  }),
```

---

## Phase 5 — Frontend UI

### Create-user form (wherever the existing "Add user" lives)

Add a `target_calories` field:

```vue
<script setup lang="ts">
const name   = ref<string>('')
const target = ref<string>('2000')

async function submit(): Promise<void> {
  const t = Number(target.value)
  if (!name.value.trim() || !Number.isFinite(t) || t <= 0) {
    errMsg.value = 'Enter a name and a positive calorie target'
    return
  }
  await api.createUser({ name: name.value.trim(), target_calories: Math.round(t) })
  // close dialog / refetch users
}
</script>

<template>
  <Input v-model="name"   placeholder="Name" />
  <Input v-model="target" type="number" inputmode="numeric" min="1" step="50" placeholder="Daily calorie target" />
</template>
```

The seeded default ("2000") gives a sensible starting point; the field is required.

### `UserPage.vue` — target input now updates the user

The visual layout doesn't change. Behaviour changes for the `target_calories` input only.

```ts
const targetCalories = ref<string>(String(user.value?.target_calories ?? ''))

async function saveTargetCalories(): Promise<void> {
  const t = Number(targetCalories.value)
  if (!Number.isFinite(t) || t <= 0) return
  const updated = await api.updateUser(props.userId, { target_calories: Math.round(t) })
  userStore.upsert(updated) // see store change below
  await loadCharts() // re-render charts with new target line
}

async function saveMetrics(): Promise<void> {
  // unchanged in shape — but no target_calories field anymore
  await api.saveMetrics(props.userId, {
    date: date.value,
    weight: parseOptionalNumber(weight.value),
    steps:  parseOptionalNumber(steps.value),
  })
  await loadCharts()
}
```

Bind the target input's `@blur` to `saveTargetCalories`. Bind weight/steps `@blur` to `saveMetrics`.

When the selected date changes, only weight/steps are re-read from `metricsRange[0]`. The target field is read from the user store instead — it's the same value regardless of date.

### Pinia user store — add `upsert`

```ts
function upsert(user: User): void {
  const idx = users.value.findIndex(u => u.id === user.id)
  if (idx >= 0) users.value[idx] = user
  else users.value.push(user)
}
```

### `ProgressCharts.vue` — accept target as a prop

It currently reads `target_calories` from each `DailyMetric`. Change to:

```ts
const props = defineProps<{ data: DailyMetric[]; target: number }>()
```

…and render the target as a single horizontal reference line at `props.target`. Call sites pass `user.target_calories`.

### Home page donut

No code change needed — `TodaySummary` still has `{ consumed, target }`; the backend now sources `target` from `users.target_calories`.

---

## Phase 6 — Testing checklist

- [ ] Fresh DB: create a user with `target_calories=2200`; assert it round-trips through `GET /api/users`.
- [ ] Legacy DB: pre-migration users get `users.target_calories=2000` after restart (regardless of whatever was in `daily_metrics.target_calories`).
- [ ] Legacy DB: `PRAGMA table_info(daily_metrics)` no longer shows `target_calories`.
- [ ] `POST /api/users` without `target_calories` → 400.
- [ ] `POST /api/users` with `target_calories <= 0` → 400.
- [ ] `PUT /api/users/{id}` with `{target_calories: 1900}` → user updated, future donuts and chart lines reflect 1900.
- [ ] `PUT /api/users/{id}/metrics` with `target_calories` in body → field ignored (or rejected — pick one and document); weight/steps still upsert correctly.
- [ ] UserPage: editing target on day A and switching to day B keeps target the same (it's profile-level, not per-day).
- [ ] Home donut shows the new target after edit.
- [ ] ProgressCharts target line is a single horizontal line.

---

## Phase 7 — Rollout order

1. **Schema + migration** (Phase 1) — verify with a copy of a real DB before merging.
2. **SQL queries + sqlc generate** (Phase 2).
3. **Backend handlers + routes + seed** (Phase 3); curl-test new endpoints.
4. **Frontend types + API client** (Phase 4).
5. **Frontend UI** (Phase 5) — new-user form, UserPage rewiring, ProgressCharts prop.
6. **Run `vue-tsc --noEmit` and `go build ./...`** after each backend/frontend phase.
7. **Smoke test** with a legacy DB to confirm existing users default to `2000` and `daily_metrics.target_calories` is gone (Phase 6).
8. **Update `research.md`** to reflect the new schema, endpoints, and design rationale.

---

## Open considerations

- **`PUT /api/users/{id}/metrics` with `target_calories` in the body**: silently ignore vs. 400. Silent-ignore is more permissive and won't break old clients; 400 catches frontend bugs faster. Recommend: silently ignore the field if present (drop the JSON tag); the field simply no longer exists on the type.
- **Per-day target overrides**: out of scope. If ever wanted, you'd reintroduce `daily_metrics.target_calories` as a nullable override, with `COALESCE(dm.target_calories, u.target_calories)` reads.
- **Removing the column on legacy SQLite**: `ALTER TABLE … DROP COLUMN` requires SQLite ≥ 3.35. `modernc.org/sqlite v1.50.x` ships a newer engine than that, so this works without the historical "rename table → recreate → copy → drop" dance.
- **Validation bounds**: `> 0` is the only constraint enforced. A sane upper bound (e.g. `< 20000`) could be added for defensive UX but isn't required.
