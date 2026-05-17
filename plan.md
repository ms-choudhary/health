# Calorie Tracker — Implementation Plan

## Overview

Multi-user calorie tracker with a shared food library, per-user daily logging, and progress charts.
Stack: Vue 3 + shadcn-vue (frontend) · Go + `net/http` ServeMux (API) · SQLite + sqlc (storage).

---

## App Flow

```
Home (/)
 ├── User card (avatar + donut chart: consumed / target cal)  →  /user/:id
 ├── User card …
 ├── + Add user
 └── Food Library →  /library

User page (/user/:id)          ← single page, two sections
 ├── Section: Log
 │    ├── Date nav  ← / date / → (→ disabled on today)
 │    ├── Food log table  (food · qty · unit · cal)
 │    ├── + Add food  →  Add Food drawer (overlay)
 │    └── Inputs: weight · steps · target calories
 └── Section: Progress (W / M / Yr)
      ├── Chart: calories consumed  +  target line
      ├── Chart: weight
      └── Chart: steps

Add Food drawer (overlay on user page)
 ├── Search bar
 ├── "Recent" — user's previously logged items (tap to re-add with same qty)
 └── Library results  (tap to pick food, enter qty)

Food Library (/library)
 ├── Search bar
 ├── Food list  (name · unit · cal / unit)
 └── + Add food item
```

---

## Screens & Routes

| # | Screen | Route |
|---|--------|-------|
| 1 | Home — user list with donut charts | `/` |
| 2 | User page — log + progress | `/user/:id` |
| 3 | Food library (shared) | `/library` |
| 4 | Add food drawer | overlay on `/user/:id` |

---

## Project Structure

```
health/
├── backend/
│   ├── main.go
│   ├── sqlc.yaml
│   ├── sql/
│   │   ├── schema.sql          ← source of truth
│   │   └── queries/
│   │       ├── users.sql
│   │       ├── foods.sql
│   │       ├── log.sql
│   │       └── metrics.sql
│   ├── db/
│   │   ├── db.go               ← open SQLite, run migrations
│   │   └── queries/            ← sqlc-generated Go code (do not edit)
│   │       ├── models.go
│   │       ├── users.sql.go
│   │       ├── foods.sql.go
│   │       ├── log.sql.go
│   │       └── metrics.sql.go
│   └── handlers/
│       ├── users.go
│       ├── foods.go
│       ├── log.go
│       └── metrics.go
│
└── frontend/
    ├── package.json
    ├── vite.config.ts
    ├── src/
    │   ├── main.ts
    │   ├── App.vue
    │   ├── router/index.ts
    │   ├── stores/user.ts          ← Pinia
    │   ├── lib/api.ts
    │   └── views/
    │       ├── Home.vue            ← user list + donut charts
    │       ├── UserPage.vue        ← log + progress (one page)
    │       └── FoodLibrary.vue     ← shared library
    └── components/
        ├── AddFoodDrawer.vue       ← search overlay
        ├── DonutChart.vue
        ├── ProgressCharts.vue
        └── ui/                     ← shadcn-vue components
```

---

## Database Schema

```sql
-- sql/schema.sql

CREATE TABLE users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL,
  avatar     TEXT NOT NULL,          -- 2-letter initials
  created_at TEXT NOT NULL DEFAULT (date('now'))
);

-- Shared food library (not per-user)
CREATE TABLE foods (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  name              TEXT NOT NULL,
  unit              TEXT NOT NULL DEFAULT 'g',   -- g | ml | oz | piece | tbsp …
  calories_per_unit REAL NOT NULL,               -- calories per 1 unit
  created_at        TEXT NOT NULL DEFAULT (date('now'))
);

CREATE TABLE log_entries (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id           INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  food_id           INTEGER REFERENCES foods(id) ON DELETE SET NULL,
  date              TEXT NOT NULL,               -- YYYY-MM-DD
  food_name         TEXT NOT NULL,               -- snapshot (survives food deletion)
  food_unit         TEXT NOT NULL,               -- snapshot
  calories_per_unit REAL NOT NULL,               -- snapshot
  quantity          REAL NOT NULL,
  calories          REAL NOT NULL                -- calories_per_unit * quantity, stored at insert
);

CREATE TABLE daily_metrics (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date            TEXT NOT NULL,                 -- YYYY-MM-DD
  weight          REAL,                          -- kg
  steps           INTEGER,
  target_calories INTEGER,                       -- user-set calorie goal for this day
  UNIQUE(user_id, date)
);

CREATE INDEX idx_log_user_date     ON log_entries(user_id, date);
CREATE INDEX idx_metrics_user_date ON daily_metrics(user_id, date);
```

---

## sqlc Setup

### `backend/sqlc.yaml`

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "sql/queries"
    schema:  "sql/schema.sql"
    gen:
      go:
        package:      "queries"
        out:          "db/queries"
        emit_json_tags: true
```

### `backend/sql/queries/foods.sql`

```sql
-- name: ListFoods :many
SELECT * FROM foods
WHERE name LIKE '%' || sqlc.arg(search) || '%'
ORDER BY name;

-- name: CreateFood :one
INSERT INTO foods (name, unit, calories_per_unit)
VALUES (?, ?, ?)
RETURNING *;

-- name: DeleteFood :exec
DELETE FROM foods WHERE id = ?;
```

### `backend/sql/queries/log.sql`

```sql
-- name: GetLogForDate :many
SELECT * FROM log_entries
WHERE user_id = ? AND date = ?
ORDER BY id;

-- name: GetRecentLoggedFoods :many
-- Most-recently-logged distinct foods by this user, for the Add Food drawer history.
SELECT DISTINCT food_name, food_unit, calories_per_unit, food_id,
       MAX(quantity) AS last_quantity
FROM log_entries
WHERE user_id = ?
GROUP BY food_name
ORDER BY MAX(id) DESC
LIMIT 20;

-- name: AddLogEntry :one
INSERT INTO log_entries
  (user_id, food_id, date, food_name, food_unit, calories_per_unit, quantity, calories)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteLogEntry :exec
DELETE FROM log_entries WHERE id = ? AND user_id = ?;
```

### `backend/sql/queries/metrics.sql`

```sql
-- name: UpsertMetrics :one
INSERT INTO daily_metrics (user_id, date, weight, steps, target_calories)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id, date) DO UPDATE SET
  weight          = excluded.weight,
  steps           = excluded.steps,
  target_calories = excluded.target_calories
RETURNING *;

-- name: GetMetricsRange :many
SELECT * FROM daily_metrics
WHERE user_id = ? AND date BETWEEN ? AND ?
ORDER BY date;

-- name: GetTodayCalories :one
-- Used for home page donut chart.
SELECT
  COALESCE(SUM(l.calories), 0) AS consumed,
  COALESCE(m.target_calories, 0) AS target
FROM daily_metrics m
LEFT JOIN log_entries l ON l.user_id = m.user_id AND l.date = m.date
WHERE m.user_id = ? AND m.date = date('now');
```

---

## Backend (`net/http` + sqlc)

### `backend/main.go`

```go
package main

import (
    "log"
    "net/http"
    "health/db"
    "health/handlers"
)

func main() {
    queries := db.Init("health.db")
    h := handlers.New(queries)

    mux := http.NewServeMux()

    // Users
    mux.HandleFunc("GET /api/users",        h.ListUsers)
    mux.HandleFunc("POST /api/users",        h.CreateUser)
    mux.HandleFunc("DELETE /api/users/{id}", h.DeleteUser)

    // Shared food library
    mux.HandleFunc("GET /api/foods",           h.ListFoods)    // ?q=
    mux.HandleFunc("POST /api/foods",          h.CreateFood)
    mux.HandleFunc("DELETE /api/foods/{id}",   h.DeleteFood)

    // Per-user log
    mux.HandleFunc("GET /api/users/{id}/log",           h.GetLog)     // ?date=
    mux.HandleFunc("POST /api/users/{id}/log",          h.AddLogEntry)
    mux.HandleFunc("DELETE /api/users/{id}/log/{eid}",  h.DeleteLogEntry)

    // Per-user recent foods (for Add Food drawer history)
    mux.HandleFunc("GET /api/users/{id}/recent-foods",  h.GetRecentFoods)

    // Per-user metrics
    mux.HandleFunc("GET /api/users/{id}/metrics",  h.GetMetrics)   // ?from=&to=
    mux.HandleFunc("PUT /api/users/{id}/metrics",  h.UpsertMetrics)

    // Today's calorie summary (for home page donut)
    mux.HandleFunc("GET /api/users/{id}/today",    h.GetTodaySummary)

    log.Println("Listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", corsMiddleware(mux)))
}

func corsMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
        if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
        next.ServeHTTP(w, r)
    })
}
```

### `backend/db/db.go`

```go
package db

import (
    "database/sql"
    "health/db/queries"
    _ "modernc.org/sqlite"   // pure-Go SQLite driver, no cgo
    "os"
)

func Init(path string) *queries.Queries {
    conn, err := sql.Open("sqlite", path)
    if err != nil { panic(err) }
    schema, _ := os.ReadFile("sql/schema.sql")
    conn.Exec(string(schema))
    return queries.New(conn)
}
```

### `backend/handlers/handlers.go`

```go
package handlers

import (
    "encoding/json"
    "health/db/queries"
    "net/http"
)

type Handler struct{ q *queries.Queries }

func New(q *queries.Queries) *Handler { return &Handler{q: q} }

func writeJSON(w http.ResponseWriter, code int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
    return json.NewDecoder(r.Body).Decode(v)
}
```

### `backend/handlers/log.go`

```go
func (h *Handler) AddLogEntry(w http.ResponseWriter, r *http.Request) {
    userID := r.PathValue("id")
    var body struct {
        FoodID          int64   `json:"food_id"`
        FoodName        string  `json:"food_name"`
        FoodUnit        string  `json:"food_unit"`
        CaloriesPerUnit float64 `json:"calories_per_unit"`
        Quantity        float64 `json:"quantity"`
        Date            string  `json:"date"`
    }
    if err := readJSON(r, &body); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
        return
    }
    calories := body.CaloriesPerUnit * body.Quantity
    entry, err := h.q.AddLogEntry(r.Context(), queries.AddLogEntryParams{
        UserID:          mustParseID(userID),
        FoodID:          body.FoodID,
        Date:            body.Date,
        FoodName:        body.FoodName,
        FoodUnit:        body.FoodUnit,
        CaloriesPerUnit: body.CaloriesPerUnit,
        Quantity:        body.Quantity,
        Calories:        calories,
    })
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, http.StatusCreated, entry)
}
```

---

## Frontend (Vue 3 + shadcn-vue)

### Setup

```bash
npm create vite@latest frontend -- --template vue-ts
cd frontend
npx shadcn-vue@latest init
npm install vue-router@4 pinia @vueuse/core date-fns lucide-vue-next
```

### Routes (`src/router/index.ts`)

```ts
import { createRouter, createWebHistory } from 'vue-router'
import Home         from '@/views/Home.vue'
import UserPage     from '@/views/UserPage.vue'
import FoodLibrary  from '@/views/FoodLibrary.vue'

export default createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/',          component: Home },
    { path: '/user/:id',  component: UserPage },
    { path: '/library',   component: FoodLibrary },
  ],
})
```

### `src/lib/api.ts`

```ts
const BASE = 'http://localhost:8080/api'
const get  = (url: string) => fetch(url).then(r => r.json())
const post = (url: string, body: object) =>
  fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' },
               body: JSON.stringify(body) }).then(r => r.json())
const put  = (url: string, body: object) =>
  fetch(url, { method: 'PUT',  headers: { 'Content-Type': 'application/json' },
               body: JSON.stringify(body) }).then(r => r.json())
const del  = (url: string) => fetch(url, { method: 'DELETE' })

export const api = {
  // Users
  users:    ()              => get(`${BASE}/users`),
  addUser:  (name: string)  => post(`${BASE}/users`, { name, avatar: name.slice(0,2).toUpperCase() }),
  delUser:  (id: number)    => del(`${BASE}/users/${id}`),

  // Today summary (for donut chart)
  today:    (id: number)    => get(`${BASE}/users/${id}/today`),

  // Shared food library
  foods:    (q = '')        => get(`${BASE}/foods?q=${encodeURIComponent(q)}`),
  addFood:  (f: object)     => post(`${BASE}/foods`, f),
  delFood:  (id: number)    => del(`${BASE}/foods/${id}`),

  // Log
  log:      (id: number, date: string) => get(`${BASE}/users/${id}/log?date=${date}`),
  addLog:   (id: number, e: object)    => post(`${BASE}/users/${id}/log`, e),
  delLog:   (id: number, eid: number)  => del(`${BASE}/users/${id}/log/${eid}`),

  // Recent foods (user history for Add Food drawer)
  recent:   (id: number)    => get(`${BASE}/users/${id}/recent-foods`),

  // Metrics
  metrics:  (id: number, from: string, to: string) =>
              get(`${BASE}/users/${id}/metrics?from=${from}&to=${to}`),
  saveMet:  (id: number, m: object) => put(`${BASE}/users/${id}/metrics`, m),
}
```

### Screen 1 — `Home.vue`

```vue
<template>
  <div class="max-w-lg mx-auto p-4 flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold">Health Tracker</h1>
      <Button variant="outline" size="sm" @click="router.push('/library')">
        <BookOpen class="h-4 w-4 mr-1" /> Food Library
      </Button>
    </div>

    <div class="flex flex-col gap-3">
      <div v-for="user in users" :key="user.id"
           class="card p-4 flex items-center gap-4 cursor-pointer hover:bg-muted"
           @click="router.push(`/user/${user.id}`)">
        <Avatar class="h-12 w-12">
          <AvatarFallback>{{ user.avatar }}</AvatarFallback>
        </Avatar>
        <div class="flex-1">
          <div class="font-semibold">{{ user.name }}</div>
          <div class="text-sm text-muted-foreground">
            {{ todayMap[user.id]?.consumed ?? 0 }} /
            {{ todayMap[user.id]?.target   ?? '—'  }} kcal today
          </div>
        </div>
        <!-- Donut chart -->
        <DonutChart
          :consumed="todayMap[user.id]?.consumed ?? 0"
          :target="todayMap[user.id]?.target ?? 0"
          :size="52"
        />
      </div>
    </div>

    <Button variant="outline" class="w-full" @click="showAddUser = true">
      + Add user
    </Button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'

const router = useRouter()
const users    = ref([])
const todayMap = ref<Record<number, { consumed: number; target: number }>>({})

onMounted(async () => {
  users.value = await api.users()
  const summaries = await Promise.all(users.value.map(u => api.today(u.id)))
  users.value.forEach((u, i) => { todayMap.value[u.id] = summaries[i] })
})
</script>
```

### `DonutChart.vue`

```vue
<template>
  <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
    <circle :cx="cx" :cy="cy" :r="r"
            fill="none" stroke="hsl(var(--muted))" :stroke-width="sw"/>
    <circle :cx="cx" :cy="cy" :r="r"
            fill="none" :stroke="color" :stroke-width="sw"
            :stroke-dasharray="`${filled} ${circumference}`"
            stroke-linecap="round"
            :transform="`rotate(-90 ${cx} ${cy})`"/>
    <text :x="cx" :y="cy + 4" text-anchor="middle"
          font-size="10" font-weight="600" fill="currentColor">
      {{ pct }}%
    </text>
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
const props = defineProps<{ consumed: number; target: number; size?: number }>()
const size  = computed(() => props.size ?? 56)
const sw    = computed(() => size.value * 0.12)
const r     = computed(() => (size.value - sw.value) / 2)
const cx    = computed(() => size.value / 2)
const cy    = computed(() => size.value / 2)
const circumference = computed(() => 2 * Math.PI * r.value)
const pct   = computed(() => props.target > 0
  ? Math.min(100, Math.round((props.consumed / props.target) * 100)) : 0)
const filled = computed(() => (pct.value / 100) * circumference.value)
const color  = computed(() =>
  pct.value >= 100 ? 'hsl(0 72% 51%)' :
  pct.value >= 80  ? 'hsl(38 92% 50%)' :
                     'hsl(221 83% 53%)')
</script>
```

### Screen 2 — `UserPage.vue` (log + progress, one page)

```vue
<template>
  <div class="max-w-lg mx-auto p-4 pb-8 flex flex-col gap-6">
    <!-- Header -->
    <div class="flex items-center gap-3">
      <Button variant="ghost" size="icon" @click="router.back()">←</Button>
      <h1 class="text-xl font-bold">{{ user?.name }}</h1>
    </div>

    <!-- ── LOG SECTION ── -->
    <section class="flex flex-col gap-3">
      <h2 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">Log</h2>

      <!-- Date nav -->
      <div class="flex items-center justify-between">
        <Button variant="ghost" size="icon" @click="prevDay">←</Button>
        <span class="font-semibold">{{ displayDate }}</span>
        <Button variant="ghost" size="icon" @click="nextDay" :disabled="isToday">→</Button>
      </div>

      <!-- Food table -->
      <Card>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Food</TableHead>
              <TableHead class="text-right">Qty</TableHead>
              <TableHead class="text-right">Cal</TableHead>
              <TableHead class="w-8"/>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="e in entries" :key="e.id">
              <TableCell>
                <div>{{ e.food_name }}</div>
                <div class="text-xs text-muted-foreground">{{ e.food_unit }}</div>
              </TableCell>
              <TableCell class="text-right">{{ e.quantity }}</TableCell>
              <TableCell class="text-right">{{ Math.round(e.calories) }}</TableCell>
              <TableCell>
                <Button variant="ghost" size="icon" @click="removeEntry(e.id)">×</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        <div class="p-3 border-t flex justify-between font-semibold">
          <span>Total</span>
          <span>{{ totalCal }} kcal</span>
        </div>
      </Card>

      <Button variant="outline" class="w-full" @click="showDrawer = true">
        + Add food
      </Button>

      <!-- Weight / Steps / Target -->
      <Card>
        <CardContent class="pt-4 flex flex-col gap-3">
          <div v-for="field in metricFields" :key="field.key" class="flex items-center gap-3">
            <label class="w-28 text-sm">{{ field.label }}</label>
            <Input v-model="metrics[field.key]" type="number"
                   :placeholder="field.placeholder" class="flex-1" @blur="saveMetrics"/>
            <span class="text-xs text-muted-foreground w-12">{{ field.unit }}</span>
          </div>
        </CardContent>
      </Card>
    </section>

    <!-- ── PROGRESS SECTION ── -->
    <section class="flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <h2 class="text-sm font-semibold text-muted-foreground uppercase tracking-wide">Progress</h2>
        <div class="flex gap-1">
          <Button v-for="p in periods" :key="p.v" size="sm"
                  :variant="period === p.v ? 'default' : 'outline'"
                  @click="period = p.v">{{ p.l }}</Button>
        </div>
      </div>
      <ProgressCharts :data="chartData" />
    </section>
  </div>

  <!-- Add Food drawer -->
  <AddFoodDrawer v-if="showDrawer" :userId="userId"
    @close="showDrawer = false"
    @add="onAddFood" />
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { format, addDays, isToday as dateFnsIsToday, subDays, subMonths, subYears } from 'date-fns'
import { api } from '@/lib/api'

const route   = useRoute()
const router  = useRouter()
const userId  = Number(route.params.id)
const user    = ref(null)
const date    = ref(format(new Date(), 'yyyy-MM-dd'))
const entries = ref([])
const metrics = ref({ weight: '', steps: '', target_calories: '' })
const chartData = ref([])
const period  = ref('w')
const periods = [{ v:'w', l:'W' }, { v:'m', l:'M' }, { v:'yr', l:'Yr' }]
const showDrawer = ref(false)

const metricFields = [
  { key: 'weight',          label: 'Weight',         placeholder: '0',    unit: 'kg'    },
  { key: 'steps',           label: 'Steps',          placeholder: '0',    unit: 'steps' },
  { key: 'target_calories', label: 'Target calories', placeholder: '2000', unit: 'kcal' },
]

const isToday    = computed(() => dateFnsIsToday(new Date(date.value + 'T00:00:00')))
const displayDate = computed(() => format(new Date(date.value + 'T00:00:00'), 'd MMM yyyy'))
const totalCal   = computed(() => Math.round(entries.value.reduce((s, e) => s + e.calories, 0)))

const prevDay = () => date.value = format(addDays(new Date(date.value + 'T00:00:00'), -1), 'yyyy-MM-dd')
const nextDay = () => { if (!isToday.value) date.value = format(addDays(new Date(date.value + 'T00:00:00'), 1), 'yyyy-MM-dd') }

const loadLog = async () => {
  entries.value = await api.log(userId, date.value)
  const m = await api.metrics(userId, date.value, date.value)
  if (m[0]) Object.assign(metrics.value, m[0])
}

const loadCharts = async () => {
  const to   = format(new Date(), 'yyyy-MM-dd')
  const from = format(
    period.value === 'w' ? subDays(new Date(), 7) :
    period.value === 'm' ? subMonths(new Date(), 1) :
                           subYears(new Date(), 1),
    'yyyy-MM-dd')
  chartData.value = await api.metrics(userId, from, to)
}

const saveMetrics = () => api.saveMet(userId, { date: date.value, ...metrics.value })

const removeEntry = async (id: number) => {
  await api.delLog(userId, id)
  await loadLog()
}

const onAddFood = async (entry: object) => {
  await api.addLog(userId, { ...entry, date: date.value })
  await loadLog()
  showDrawer.value = false
}

watch(date, loadLog)
watch(period, loadCharts)
onMounted(async () => {
  // load user name for header
  const all = await api.users()
  user.value = all.find(u => u.id === userId)
  await loadLog()
  await loadCharts()
})
</script>
```

### `AddFoodDrawer.vue`

```vue
<template>
  <!-- Bottom sheet overlay -->
  <div class="fixed inset-0 z-50" @click.self="$emit('close')">
    <div class="absolute bottom-0 left-0 right-0 max-w-lg mx-auto
                bg-background border-t rounded-t-2xl p-4 flex flex-col gap-4
                max-h-[80vh] overflow-y-auto">
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">Add food</h2>
        <Button variant="ghost" size="icon" @click="$emit('close')">×</Button>
      </div>

      <!-- Search -->
      <div class="relative">
        <Input v-model="query" placeholder="Search food library…" @input="onSearch"/>
        <Search class="absolute right-3 top-2.5 h-4 w-4 text-muted-foreground"/>
      </div>

      <!-- Quantity input (shown after picking a food) -->
      <template v-if="picked">
        <div class="flex items-center gap-3 p-3 bg-muted rounded-lg">
          <div class="flex-1">
            <div class="font-medium">{{ picked.food_name }}</div>
            <div class="text-xs text-muted-foreground">
              {{ picked.calories_per_unit }} kcal / {{ picked.food_unit }}
            </div>
          </div>
          <Input v-model.number="qty" type="number" :placeholder="String(picked.last_quantity ?? 1)"
                 class="w-24" min="0.1" step="0.1"/>
          <span class="text-sm">{{ picked.food_unit }}</span>
          <Button @click="confirmAdd">Add</Button>
          <Button variant="ghost" @click="picked = null">×</Button>
        </div>
      </template>

      <!-- Recent items (user history) -->
      <template v-if="!picked && !query">
        <div class="text-xs font-semibold text-muted-foreground uppercase">Recent</div>
        <div v-for="item in recent" :key="item.food_name"
             class="flex items-center justify-between p-3 border rounded-lg cursor-pointer hover:bg-muted"
             @click="pickItem(item)">
          <div>
            <div class="text-sm font-medium">{{ item.food_name }}</div>
            <div class="text-xs text-muted-foreground">
              {{ item.last_quantity }} {{ item.food_unit }} · {{ Math.round(item.calories_per_unit * item.last_quantity) }} kcal
            </div>
          </div>
          <Badge variant="secondary">{{ Math.round(item.calories_per_unit * item.last_quantity) }}</Badge>
        </div>
      </template>

      <!-- Library search results -->
      <template v-if="!picked && query">
        <div class="text-xs font-semibold text-muted-foreground uppercase">Library</div>
        <div v-for="food in results" :key="food.id"
             class="flex items-center justify-between p-3 border rounded-lg cursor-pointer hover:bg-muted"
             @click="pickLibraryItem(food)">
          <div>
            <div class="text-sm font-medium">{{ food.name }}</div>
            <div class="text-xs text-muted-foreground">{{ food.calories_per_unit }} kcal / {{ food.unit }}</div>
          </div>
          <Badge variant="outline">{{ food.unit }}</Badge>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/lib/api'

const props = defineProps<{ userId: number }>()
const emit  = defineEmits(['close', 'add'])

const query   = ref('')
const results = ref([])
const recent  = ref([])
const picked  = ref(null)
const qty     = ref(null)

onMounted(async () => { recent.value = await api.recent(props.userId) })

const onSearch = async () => {
  if (query.value.trim()) results.value = await api.foods(query.value)
}

const pickItem = (item) => {         // from recent history
  picked.value = item
  qty.value = item.last_quantity
}

const pickLibraryItem = (food) => { // from library search
  picked.value = { food_id: food.id, food_name: food.name,
                   food_unit: food.unit, calories_per_unit: food.calories_per_unit,
                   last_quantity: 1 }
  qty.value = 1
}

const confirmAdd = () => {
  if (!qty.value || qty.value <= 0) return
  emit('add', {
    food_id:          picked.value.food_id,
    food_name:        picked.value.food_name,
    food_unit:        picked.value.food_unit,
    calories_per_unit: picked.value.calories_per_unit,
    quantity:         qty.value,
  })
}
</script>
```

### Screen 3 — `FoodLibrary.vue`

```vue
<template>
  <div class="max-w-lg mx-auto p-4 flex flex-col gap-4">
    <div class="flex items-center gap-3">
      <Button variant="ghost" size="icon" @click="router.back()">←</Button>
      <h1 class="text-xl font-bold">Food Library</h1>
    </div>

    <div class="relative">
      <Input v-model="query" placeholder="Search…" @input="search"/>
      <Search class="absolute right-3 top-2.5 h-4 w-4 text-muted-foreground"/>
    </div>

    <div class="flex flex-col gap-2">
      <div v-for="food in foods" :key="food.id"
           class="flex items-center justify-between p-3 border rounded-xl">
        <div>
          <div class="font-medium">{{ food.name }}</div>
          <div class="text-sm text-muted-foreground">
            {{ food.calories_per_unit }} kcal / 1 {{ food.unit }}
          </div>
        </div>
        <div class="flex items-center gap-2">
          <Badge variant="secondary">{{ food.unit }}</Badge>
          <Button variant="ghost" size="icon" @click="deleteFood(food.id)">
            <Trash2 class="h-4 w-4"/>
          </Button>
        </div>
      </div>
    </div>

    <!-- Add food dialog -->
    <Dialog v-model:open="showAdd">
      <DialogTrigger as-child>
        <Button class="w-full">+ Add food</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader><DialogTitle>New food</DialogTitle></DialogHeader>
        <div class="flex flex-col gap-3">
          <Input v-model="newFood.name"              placeholder="Food name"/>
          <div class="flex gap-2">
            <Input v-model.number="newFood.calories" placeholder="Calories" type="number" class="flex-1"/>
            <select v-model="newFood.unit" class="border rounded-md px-3 text-sm">
              <option v-for="u in units" :key="u" :value="u">{{ u }}</option>
            </select>
          </div>
          <p class="text-xs text-muted-foreground">
            Calories per 1 {{ newFood.unit }}
          </p>
          <Button @click="addFood">Add to library</Button>
        </div>
      </DialogContent>
    </Dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/lib/api'

const query   = ref('')
const foods   = ref([])
const showAdd = ref(false)
const units   = ['g', 'ml', 'oz', 'piece', 'tbsp', 'cup', 'serving']
const newFood = ref({ name: '', calories: null, unit: 'g' })

const search = async () => { foods.value = await api.foods(query.value) }
const addFood = async () => {
  await api.addFood({ name: newFood.value.name,
                      calories_per_unit: newFood.value.calories,
                      unit: newFood.value.unit })
  showAdd.value = false
  newFood.value = { name: '', calories: null, unit: 'g' }
  await search()
}
const deleteFood = async (id: number) => { await api.delFood(id); await search() }

onMounted(search)
</script>
```

### `ProgressCharts.vue`

```vue
<template>
  <div class="flex flex-col gap-4">
    <!-- Calories: consumed line + target dashed line -->
    <Card>
      <CardHeader class="pb-2">
        <CardTitle class="text-sm">Calories</CardTitle>
      </CardHeader>
      <CardContent>
        <Line :data="calorieData" :options="lineOpts"/>
      </CardContent>
    </Card>
    <!-- Weight -->
    <Card>
      <CardHeader class="pb-2"><CardTitle class="text-sm">Weight (kg)</CardTitle></CardHeader>
      <CardContent><Line :data="weightData" :options="lineOpts"/></CardContent>
    </Card>
    <!-- Steps -->
    <Card>
      <CardHeader class="pb-2"><CardTitle class="text-sm">Steps</CardTitle></CardHeader>
      <CardContent><Bar :data="stepsData" :options="barOpts"/></CardContent>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Line, Bar } from 'vue-chartjs'
// ... Chart.js registrations omitted for brevity

const props = defineProps<{ data: any[] }>()

const calorieData = computed(() => ({
  labels: props.data.map(d => d.date),
  datasets: [
    { label: 'Consumed', data: props.data.map(d => d.calories_consumed),
      borderColor: '#3b82f6', tension: 0.3, fill: false },
    { label: 'Target',   data: props.data.map(d => d.target_calories),
      borderColor: '#f59e0b', borderDash: [5,5], tension: 0, fill: false },
  ]
}))
</script>
```

---

## API Endpoints Summary

| Method | Path | Description |
|--------|------|-------------|
| GET    | `/api/users` | List all users |
| POST   | `/api/users` | Create user |
| DELETE | `/api/users/{id}` | Delete user |
| GET    | `/api/users/{id}/today` | Today's consumed + target (for donut) |
| GET    | `/api/foods?q=` | Search shared food library |
| POST   | `/api/foods` | Add food to library |
| DELETE | `/api/foods/{id}` | Remove food |
| GET    | `/api/users/{id}/log?date=` | Get daily log entries |
| POST   | `/api/users/{id}/log` | Add log entry |
| DELETE | `/api/users/{id}/log/{eid}` | Remove log entry |
| GET    | `/api/users/{id}/recent-foods` | User's recent food history (drawer) |
| GET    | `/api/users/{id}/metrics?from=&to=` | Get metrics range |
| PUT    | `/api/users/{id}/metrics` | Upsert weight / steps / target calories |

---

## Setup Commands

```bash
# Backend
cd backend
go mod init health
go get modernc.org/sqlite          # pure-Go SQLite, no cgo needed
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
go run main.go

# Frontend
cd frontend
npm create vite@latest . -- --template vue-ts
npx shadcn-vue@latest init
npm install vue-router@4 pinia @vueuse/core date-fns lucide-vue-next vue-chartjs chart.js
npm run dev
```

---

## Key Design Decisions

- **Shared food library**: all users see and add to one food library; logs are per-user
- **User history in Add Food drawer**: shows the current user's previously logged items first (with last-used quantity pre-filled), for speed; full library search is one tap away
- **calories stored at insert**: `calories = calories_per_unit × quantity` is calculated in the handler and stored — avoids recomputing if food definition changes later
- **Food snapshots in log**: `food_name`, `food_unit`, `calories_per_unit` copied into `log_entries` so old logs remain accurate if a food is edited/deleted
- **target_calories per day** on `daily_metrics`: lets different days have different goals; used as the dashed target line in the calorie chart and the donut on home
- **sqlc**: compile-time SQL → no ORM magic, queries are plain `.sql` files, generated Go structs are type-safe
- **`net/http` ServeMux (Go 1.22+)**: supports `GET /path/{id}` method+path routing natively, zero dependencies
- **`modernc.org/sqlite`**: pure-Go SQLite driver, no CGO, easy cross-compilation
- **Single user page** (`/user/:id`): log and progress live on one scrollable page — fewer navigation steps on mobile
- **Date as TEXT (YYYY-MM-DD)**: avoids SQLite timezone bugs; sorts lexicographically
- **"→" disabled if today**: prevents logging into future dates

---

## Implementation Todo

Phases are ordered so each one produces something testable end-to-end before moving on. Aim to land each phase as its own commit.

### Phase 0 · Repo scaffolding
- [ ] Initialize git repo, add `.gitignore` (Go binary, `node_modules`, `health.db`, `dist/`)
- [ ] Create `backend/` and `frontend/` top-level directories
- [ ] Add root `README.md` with one-line description + how to run both sides
- [ ] Decide port convention (backend `:8080`, frontend dev `:5173`) and document it

### Phase 1 · Backend foundations
- [ ] `go mod init health` inside `backend/`
- [ ] Add deps: `modernc.org/sqlite` (driver), install `sqlc` CLI globally
- [ ] Write `sql/schema.sql` (users, foods, log_entries, daily_metrics + indexes)
- [ ] Write `sqlc.yaml` (sqlite engine, output to `db/queries`)
- [ ] Write `db/db.go` — `Init(path)` opens conn, executes `schema.sql` (idempotent via `CREATE TABLE IF NOT EXISTS`)
- [ ] Verify `sqlc generate` produces compilable Go in `db/queries/`
- [ ] Add `main.go` skeleton with mux + CORS middleware + a `/health` ping route
- [ ] Manually hit `/health` with curl to confirm server boots

### Phase 2 · Backend — Users API
- [ ] Write `sql/queries/users.sql` (`ListUsers`, `CreateUser`, `DeleteUser`, `GetUser`)
- [ ] `sqlc generate`, verify generated code
- [ ] `handlers/handlers.go` — `Handler` struct, `writeJSON`, `readJSON`, `mustParseID` helpers
- [ ] `handlers/users.go` — list / create / delete handlers
- [ ] Wire routes in `main.go`
- [ ] Manually test with curl: create 2 users, list, delete one

### Phase 3 · Backend — Shared Food Library API
- [ ] Write `sql/queries/foods.sql` (`ListFoods` with `LIKE` search, `CreateFood`, `DeleteFood`, `GetFood`)
- [ ] `sqlc generate`
- [ ] `handlers/foods.go` — search / create / delete
- [ ] Wire routes
- [ ] curl test: add foods with `g`, `ml`, `piece` units; search with `?q=`

### Phase 4 · Backend — Log API
- [ ] Write `sql/queries/log.sql`:
  - [ ] `GetLogForDate` (user_id + date)
  - [ ] `AddLogEntry` (snapshots food fields, stores computed `calories`)
  - [ ] `DeleteLogEntry`
  - [ ] `GetRecentLoggedFoods` (DISTINCT by food_name, ordered by most recent, LIMIT 20)
- [ ] `sqlc generate`
- [ ] `handlers/log.go` — handler computes `calories = calories_per_unit × quantity` server-side
- [ ] `handlers/recent.go` — recent-foods endpoint for the Add Food drawer
- [ ] Wire routes (including `/api/users/{id}/recent-foods`)
- [ ] curl test: add log entries, fetch by date, verify recent-foods shape

### Phase 5 · Backend — Metrics & Today summary API
- [ ] Write `sql/queries/metrics.sql`:
  - [ ] `UpsertMetrics` (ON CONFLICT(user_id, date) DO UPDATE)
  - [ ] `GetMetricsRange`
  - [ ] `GetTodaySummary` (joins log_entries + daily_metrics for today's consumed + target)
- [ ] `sqlc generate`
- [ ] `handlers/metrics.go` — upsert / range / today
- [ ] Wire routes (`PUT /api/users/{id}/metrics`, `GET /api/users/{id}/today`)
- [ ] curl test: set weight/steps/target, fetch range, fetch today summary

### Phase 6 · Backend polish
- [ ] Validation: reject negative quantities, empty food names, invalid dates (regex `YYYY-MM-DD`)
- [ ] Consistent error JSON: `{ "error": "..." }`
- [ ] Log requests in dev (simple middleware)
- [ ] Seed script `cmd/seed/main.go` — inserts 2 users, ~10 foods, a week of log entries (handy for frontend dev)
- [ ] `go build` cross-platform check (Linux + macOS)

### Phase 7 · Frontend foundations
- [ ] `npm create vite@latest frontend -- --template vue-ts`
- [ ] Install deps: `vue-router@4`, `pinia`, `@vueuse/core`, `date-fns`, `lucide-vue-next`, `vue-chartjs`, `chart.js`
- [ ] `npx shadcn-vue@latest init` — pick neutral colour scheme
- [ ] Configure Vite proxy `/api → :8080` so frontend doesn't need full URLs in dev
- [ ] Set up Tailwind config (shadcn-vue handles this) + base CSS variables
- [ ] Add `src/lib/api.ts` (single source for fetch wrappers)
- [ ] Configure router with 3 routes (`/`, `/user/:id`, `/library`)
- [ ] Confirm dev server boots, hot-reload works

### Phase 8 · Frontend — shared components
- [ ] Install required shadcn-vue primitives: `button`, `input`, `card`, `table`, `dialog`, `badge`, `avatar`
- [ ] `components/DonutChart.vue` — SVG, colour-coded by % of target, accessible (aria-label with %)
- [ ] `components/ProgressCharts.vue` — wraps Chart.js Line + Bar, takes `data` prop, period-agnostic
- [ ] `stores/user.ts` (Pinia) — currently selected user, cached user list

### Phase 9 · Frontend — Home page
- [ ] `views/Home.vue` — fetch users + today summary for each
- [ ] User card layout matching preview (avatar, name, "x / y kcal today", donut)
- [ ] "+ Add user" dialog — name input, calls `api.addUser`
- [ ] "Food Library" button → routes to `/library`
- [ ] Tap user → routes to `/user/:id`
- [ ] Empty state: "No users yet — tap Add user to start"
- [ ] Loading skeletons for cards (avoid flash)

### Phase 10 · Frontend — Food Library page
- [ ] `views/FoodLibrary.vue` — list + search input (debounced 200ms)
- [ ] Add-food dialog: name, calories (number), unit (select: g/ml/oz/piece/tbsp/cup/serving)
- [ ] Delete button per row with confirm
- [ ] Empty state: "No foods yet — tap + Add food"
- [ ] Back button → home

### Phase 11 · Frontend — User page (log section)
- [ ] `views/UserPage.vue` skeleton with header + Log section + placeholder for Progress
- [ ] Date navigation (← date →), arrow → disabled when `isToday`
- [ ] Fetch + render log entries for current date
- [ ] Total calorie row at bottom of table
- [ ] Delete entry button (with optimistic update)
- [ ] Weight / Steps / Target Calories inputs with `@blur` → `saveMetrics`
- [ ] Pre-fill inputs from existing metrics for the date
- [ ] Reactive: changing date refetches log + metrics

### Phase 12 · Frontend — Add Food drawer
- [ ] `components/AddFoodDrawer.vue` — bottom-sheet overlay, fixed positioning, scroll-locked body
- [ ] State A (no search, no pick): show "Recent" — call `api.recent(userId)`, render last-quantity meta
- [ ] State B (search active): query `api.foods(q)`, debounce 200ms, show library results
- [ ] State C (item picked): quantity input (pre-filled with `last_quantity` for recent, else 1), Add button
- [ ] Tap recent item → State C with food snapshot
- [ ] Tap library item → State C with library record + qty 1
- [ ] On Add → emit event to parent → POST log entry → close drawer
- [ ] Escape key + backdrop tap → close
- [ ] "Create new food" link if search has zero results → opens library add dialog

### Phase 13 · Frontend — User page (progress section)
- [ ] Period toggle component (W / M / Yr) styled as segmented control
- [ ] Compute `from`/`to` based on period using date-fns
- [ ] Fetch `api.metrics(userId, from, to)` on period change
- [ ] Aggregate per-day calories from log entries (backend should return this; add a query if missing)
- [ ] Calorie chart: consumed line + dashed target line (Chart.js)
- [ ] Weight chart: line
- [ ] Steps chart: bar
- [ ] Sensible Y-axis bounds (don't start at 0 for weight)
- [ ] Empty state: "Not enough data — log a few days to see progress"

### Phase 14 · Responsive & polish
- [ ] iPhone SE (375px) — everything fits, no horizontal scroll
- [ ] iPad (768–1024px) — content centred, comfortable max-width
- [ ] Mac (≥1280px) — content stays centred; consider a sidebar nav variant (deferred)
- [ ] Touch targets ≥44px on mobile
- [ ] Dark mode (shadcn-vue gives this near-free) — verify chart colours work
- [ ] Loading and error states for every async view
- [ ] Confirm `viewport` meta + iOS safe-area insets for the drawer

### Phase 15 · Quality & shipping
- [ ] Frontend: `vue-tsc` type check passes, `npm run build` succeeds
- [ ] Backend: `go vet ./...` clean, `go test ./...` (add a handful of handler tests)
- [ ] Manual smoke test of the full happy path:
  - [ ] Add user → set target calories → add foods to library → log meals → see donut update
  - [ ] Navigate back to home → donut reflects today's intake
  - [ ] Switch period on charts → data refetches
- [ ] Build single binary: `go build` produces backend; frontend builds to `dist/`; backend serves `dist/` as static files in prod
- [ ] Document run instructions in `README.md`
- [ ] Tag `v0.1.0`

### Stretch (post-v0.1)
- [ ] Edit food in library (update name/calories without losing log snapshots)
- [ ] PWA manifest + service worker for offline use
- [ ] Authentication (single-household app currently assumes trust)
