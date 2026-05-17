# Health Tracker — Deep Codebase Research Report

## 1. Project Purpose & Overview

**Health** is a full-stack, multi-user household calorie tracking application. It is built without a cloud dependency, designed to run as a single self-contained Docker container on a home server or local machine. Multiple family members ("users") share one instance; each user independently logs meals, records daily weight/steps, sets calorie targets, and views progress charts. There is no authentication — the app trusts everyone on the same network.

Key design philosophy: minimal dependencies, zero CGO, single binary deployment, type-safe database access via code generation.

---

## 2. Tech Stack

| Layer | Technology |
|-------|------------|
| Backend language | Go 1.25 |
| HTTP server | `net/http` stdlib (Go 1.22+ routing syntax) |
| Database | SQLite via `modernc.org/sqlite` (pure-Go, no CGO) |
| SQL code generation | `sqlc` |
| Frontend framework | Vue 3 (Composition API, TypeScript) |
| State management | Pinia |
| Routing | Vue Router 4 |
| Charts | Chart.js 4 via vue-chartjs |
| Styling | Tailwind CSS 3 + shadcn-vue component primitives |
| Icons | lucide-vue-next |
| Date utilities | date-fns 4 |
| Build tool | Vite 8 |
| Containerisation | Docker (multi-stage) |
| CI/CD | GitHub Actions → GHCR |
| AI integration | Google Gemini 2.5 Flash (optional) |

---

## 3. Repository Layout

```
health/
├── backend/
│   ├── main.go                  # HTTP server, routing, middleware, SPA fallback
│   ├── go.mod / go.sum          # Go module + deps
│   ├── sqlc.yaml                # sqlc code-gen config
│   ├── db/
│   │   ├── db.go                # SQLite init, schema loading
│   │   └── queries/             # sqlc-generated Go code (do not hand-edit)
│   ├── handlers/
│   │   ├── handlers.go          # Shared request/response helpers
│   │   ├── users.go
│   │   ├── foods.go
│   │   ├── log.go
│   │   ├── metrics.go
│   │   └── ai.go
│   ├── sql/
│   │   ├── schema.sql           # Table definitions
│   │   └── queries/             # Hand-written SQL (input to sqlc)
│   │       ├── users.sql
│   │       ├── foods.sql
│   │       ├── log.sql
│   │       └── metrics.sql
│   └── cmd/seed/main.go         # Dev seed script
├── frontend/
│   ├── src/
│   │   ├── main.ts              # Vue app bootstrap
│   │   ├── App.vue              # Root component (RouterView only)
│   │   ├── router/index.ts      # Route definitions
│   │   ├── stores/user.ts       # Pinia user store
│   │   ├── lib/
│   │   │   ├── api.ts           # Typed HTTP client for all endpoints
│   │   │   ├── types.ts         # TypeScript interfaces
│   │   │   └── utils.ts         # cn() helper, formatNumber
│   │   ├── views/
│   │   │   ├── Home.vue         # User list + donut charts
│   │   │   ├── UserPage.vue     # Log table + metrics + progress charts
│   │   │   └── FoodLibrary.vue  # Searchable food library + AI hint
│   │   └── components/
│   │       ├── DonutChart.vue
│   │       ├── ProgressCharts.vue
│   │       ├── AddFoodDrawer.vue
│   │       └── ui/              # shadcn-vue primitives
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── tsconfig.json
│   └── package.json
├── Dockerfile                   # Multi-stage: node → golang → alpine
├── .github/workflows/build.yml  # CI: build + push to GHCR
├── README.md
└── plan.md                      # Original implementation spec
```

---

## 4. Database Schema

### `users`
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `name` | TEXT NOT NULL | Display name |
| `avatar` | TEXT NOT NULL | 2-letter initials (auto-generated) |
| `created_at` | TEXT | YYYY-MM-DD (SQLite `date('now')`) |

### `foods` — shared food library
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `name` | TEXT NOT NULL | |
| `unit` | TEXT DEFAULT 'g' | One of: g, ml, oz, piece, tbsp, cup, serving |
| `calories_per_unit` | REAL NOT NULL | |
| `created_at` | TEXT | YYYY-MM-DD |

### `log_entries` — per-user daily food logs
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `user_id` | INTEGER FK → users | |
| `food_id` | INTEGER FK → foods (nullable) | Set to NULL when food deleted |
| `date` | TEXT | YYYY-MM-DD |
| `food_name` | TEXT | Snapshot at log time |
| `food_unit` | TEXT | Snapshot at log time |
| `calories_per_unit` | REAL | Snapshot at log time |
| `quantity` | REAL | Amount consumed |
| `calories` | REAL | Pre-computed: `calories_per_unit × quantity` |

Index: `(user_id, date)`

### `daily_metrics` — per-user daily health stats
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `user_id` | INTEGER FK → users | |
| `date` | TEXT | YYYY-MM-DD |
| `weight` | REAL (nullable) | kg |
| `steps` | INTEGER (nullable) | |
| `target_calories` | INTEGER (nullable) | |

Unique constraint: `(user_id, date)` — one row per user per day, upserted.  
Index: `(user_id, date)`

---

## 5. API Endpoints

All responses wrap data as `{ "data": ... }` or errors as `{ "error": "..." }`.

| Method | Path | Body / Query | Response | Notes |
|--------|------|-------------|----------|-------|
| GET | `/api/health` | — | `{"ok":true}` | Liveness check |
| GET | `/api/users` | — | `User[]` | All users |
| POST | `/api/users` | `{name}` | `User` | Avatar auto-generated from name initials |
| DELETE | `/api/users/{id}` | — | 204 | Cascades to log_entries + daily_metrics |
| GET | `/api/users/{id}/today` | — | `TodaySummary` | `{consumed, target}` for today |
| GET | `/api/foods` | `?q=search` | `Food[]` | LIKE search on name |
| POST | `/api/foods` | `{name, unit, calories_per_unit}` | `Food` | Add to shared library |
| DELETE | `/api/foods/{id}` | — | 204 | Nullifies food_id in log_entries |
| GET | `/api/users/{id}/log` | `?date=YYYY-MM-DD` | `LogEntry[]` | All entries for date |
| POST | `/api/users/{id}/log` | `{food_id, food_name, food_unit, calories_per_unit, quantity, date}` | `LogEntry` | Calories computed server-side |
| DELETE | `/api/users/{id}/log/{eid}` | — | 204 | Verifies ownership (user_id matches) |
| GET | `/api/users/{id}/recent-foods` | — | `RecentFood[]` | Recently logged foods with `last_quantity` |
| GET | `/api/users/{id}/metrics` | `?from=YYYY-MM-DD&to=YYYY-MM-DD` | `DailyMetric[]` | Includes `calories_consumed` aggregate per day |
| PUT | `/api/users/{id}/metrics` | `{date, weight?, steps?, target_calories?}` | `DailyMetric` | Upsert (INSERT OR REPLACE) |
| POST | `/api/ai/calorie-hint` | `{name}` | `{hint}` | Gemini call; 503 if no API key |

### Validation rules (backend)
- Date strings: must match `^\d{4}-\d{2}-\d{2}$`
- `quantity` > 0
- `calories_per_unit` >= 0
- Food name/unit: trimmed, non-empty
- All IDs: must be > 0

---

## 6. Backend Architecture

### Server (`main.go`)
- Uses Go 1.22's enhanced `http.ServeMux` which supports method prefixes and path parameters (`GET /api/users/{id}`), eliminating the need for a third-party router.
- Middleware chain: CORS headers (allow-all for dev convenience) → request logging → route dispatch.
- All `/api/*` routes are registered explicitly. Every other path falls through to the SPA handler which serves `index.html` for client-side routing.
- Static files (compiled frontend) are served from a `./frontend/dist/` path relative to the binary (resolved at startup using `os.Executable()`).

### Database (`db/db.go`)
- Single SQLite file; path configurable via `HEALTH_DB` env var (default `health.db`).
- Schema is embedded and run at startup with `CREATE TABLE IF NOT EXISTS` semantics — idempotent, no migration framework needed.
- `modernc.org/sqlite` is a pure-Go port of SQLite; `CGO_ENABLED=0` works, enabling fully static builds and simple Docker cross-compilation.

### SQL Code Generation (`sqlc`)
- All SQL lives in `/backend/sql/queries/*.sql` with `-- name: FunctionName :one/:many/:exec` annotations.
- `sqlc generate` produces type-safe Go structs and method signatures in `/backend/db/queries/`. Generated files are committed to the repo.
- Handlers import the generated `db` package and call methods directly — no string queries in handler code.

### Handlers (`handlers/`)
- Shared helpers in `handlers.go`: `writeJSON`, `writeError`, `parseID`, `parseDate`, `decodeBody`.
- Each domain (users, foods, log, metrics, ai) has its own file. Handlers are plain functions, not methods; they receive `*db.Queries` via closure or parameter.
- AI handler constructs a Gemini REST call manually (no SDK); prompt is `"ANSWER IN ONE LINE: how much calorie in {food_name} ?"` and returns the first candidate text.

---

## 7. Frontend Architecture

### Routing (`router/index.ts`)
Three routes:
- `/` → `Home.vue` (lazy-loaded)
- `/user/:id` → `UserPage.vue` (lazy-loaded)
- `/library` → `FoodLibrary.vue` (lazy-loaded)

### State (`stores/user.ts`)
Minimal Pinia store: holds the users array, exposes `load()` (fetches `/api/users`) and `findById(id)`. UserPage and Home both use this store.

### API Client (`lib/api.ts`)
A typed wrapper around `fetch`. Every endpoint has a corresponding typed function. All responses are unwrapped from `{ data: ... }`. Errors throw a JavaScript `Error` with the backend's error message.

### Views

**`Home.vue`**: Fetches all users, then for each calls `/api/users/{id}/today` to get today's calorie progress. Renders a `DonutChart` per user as a card grid. Navigation: clicking a card goes to `/user/:id`.

**`UserPage.vue`**: The most complex view.
- Date picker (defaults today) — fetches and displays log entries in a table.
- Metrics panel: weight, steps, target calories input fields with debounced PUT.
- "Add Food" button opens `AddFoodDrawer`.
- Period toggle (W / M / Yr) triggers a metrics range fetch, feeding `ProgressCharts`.

**`FoodLibrary.vue`**: Search bar with debounce against `/api/foods?q=...`. List with delete buttons. "Add Food" dialog with name/unit/calories fields. "✨ AI" button calls `/api/ai/calorie-hint` and displays the result as a suggestion.

### Components

**`DonutChart.vue`**: Pure SVG. Draws two concentric arcs (consumed vs target) using `stroke-dasharray` / `stroke-dashoffset`. Color-coded: green (under target), amber (near target), red (over target). Accepts `consumed` and `target` props; handles zero-target gracefully.

**`ProgressCharts.vue`**: Wraps Chart.js Line chart (calories vs target) and Bar chart (weight, steps) using vue-chartjs. Receives `DailyMetric[]`; formats dates with `date-fns`. Responsive via Chart.js `maintainAspectRatio: false`.

**`AddFoodDrawer.vue`**: Slide-up overlay (shadcn Sheet). Tab-like sections: "Recent Foods" (from `/api/users/{id}/recent-foods`) and "Search Library". Selecting a food shows a quantity input pre-populated with `last_quantity`. Confirm emits the log entry data to parent.

**`ui/`**: Thin wrappers around Radix Vue primitives (Avatar, Badge, Button, Card, Dialog, Input) with Tailwind variant classes — the shadcn-vue pattern.

### Styling
Tailwind 3 with CSS custom properties for theming:
```css
--background, --foreground, --primary, --muted, etc.
```
These are HSL values, allowing a dark mode toggle by swapping the `.dark` class on `<html>`. Fonts, radius, and spacing all use variables, making the design consistent and easy to reskin.

---

## 8. Key Design Decisions & Rationale

### Food Snapshots in Log Entries
When a meal is logged, `food_name`, `food_unit`, and `calories_per_unit` are copied into `log_entries`. This means:
- Editing or deleting a food from the library does not corrupt historical logs.
- `food_id` is set to NULL when the food is deleted, preserving full data integrity.

### Pre-computed `calories` Column
`calories = calories_per_unit × quantity` is computed at insert time and stored. Avoids recomputation in queries and keeps aggregation (SUM) simple.

### Date as TEXT (YYYY-MM-DD)
SQLite has no native date type. Using `TEXT` in ISO format allows lexicographic sorting (`ORDER BY date`) and range queries (`WHERE date BETWEEN '...' AND '...'`) to work correctly. It also avoids all timezone conversion issues.

### No ORM — sqlc Instead
sqlc validates SQL at code-gen time (catches typos, wrong column names) and produces idiomatic Go. No runtime query building, no reflection overhead, no magic.

### Pure-Go SQLite
`modernc.org/sqlite` is a transpiled version of SQLite's C code to Go. It enables `CGO_ENABLED=0`, which simplifies Docker multi-stage builds (no C toolchain needed in the builder) and enables fully static binaries.

### Go 1.22 stdlib Router
No Gorilla Mux, no Chi, no Gin. `http.ServeMux` in Go 1.22+ supports `METHOD /path/{param}` patterns natively. Zero added dependencies for routing.

### Single Binary + Embedded Frontend
The production binary serves both the REST API and the compiled frontend. The frontend `dist/` folder is expected adjacent to the binary (or embedded). This makes the Docker image a single artifact: one container, one port, no nginx sidecar.

### No Authentication
Explicitly out of scope. The app is intended for a single household on a trusted private network. Adding auth would significantly increase complexity for minimal benefit in this context.

### AI as Optional Enhancement
The Gemini integration is fully opt-in. If `GEMINI_API_KEY` is not set, the endpoint returns 503 and the rest of the app is completely unaffected. The frontend shows the AI button only in the Food Library, not in the critical log flow.

---

## 9. Data Flow Walkthrough

### Logging a Meal
1. User opens `UserPage` → clicks "Add Food"
2. `AddFoodDrawer` opens, fetches `recent-foods` for the user
3. User searches the food library or picks a recent food
4. User adjusts quantity → clicks "Add"
5. Frontend POSTs to `/api/users/{id}/log` with food details + quantity + date
6. Backend computes `calories`, inserts into `log_entries`, returns full entry
7. `UserPage` refetches log for the selected date → table updates
8. Home donut charts will reflect new total on next load

### Viewing Progress
1. User toggles period (W / M / Yr) on `UserPage`
2. Frontend computes `from`/`to` dates using `date-fns`
3. GET `/api/users/{id}/metrics?from=...&to=...`
4. Backend SQL joins `daily_metrics` with SUM of `log_entries.calories` per date → returns `DailyMetric[]` with `calories_consumed` filled
5. `ProgressCharts` receives the array → Chart.js renders line (calories) and bar (weight/steps) graphs

### AI Calorie Hint
1. User types a food name in `FoodLibrary` → clicks "✨ AI"
2. Frontend POSTs `{name}` to `/api/ai/calorie-hint`
3. Backend constructs Gemini REST request: `POST https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent`
4. Prompt: `"ANSWER IN ONE LINE: how much calorie in {food_name} ?"`
5. Response first candidate text returned as `{hint}`
6. Frontend displays it in a muted info box beneath the form

---

## 10. Deployment

### Development
```bash
# Terminal 1
cd backend && go run .          # API on :8080

# Terminal 2
cd frontend && npm run dev       # Vite dev server on :5173, proxies /api → :8080
```

### Production (Single Binary)
```bash
cd frontend && npm run build
cd ../backend && go build -o health
GEMINI_API_KEY=xxx ./health     # Serves everything on :8080
```

### Docker
```bash
docker build -t health:latest .
docker run -p 8080:8080 -e GEMINI_API_KEY=xxx -v /data/health:/app health:latest
```

The Dockerfile is a three-stage build:
1. `node:22-alpine` — `npm ci && npm run build` → `dist/`
2. `golang:1.25-alpine` — `go build` → static binary
3. `alpine:3.21` — copies binary + dist, sets `WORKDIR /app`, `EXPOSE 8080`

### CI/CD (`.github/workflows/build.yml`)
- Triggers on push to any branch
- Authenticates to GHCR using `GITHUB_TOKEN`
- Builds with Docker Buildx + GitHub Actions cache (`cache-from: type=gha`)
- Tags: `ghcr.io/{owner}/health:{branch_name}`

### Environment Variables
| Variable | Default | Purpose |
|----------|---------|---------|
| `HEALTH_DB` | `health.db` | SQLite file path |
| `GEMINI_API_KEY` | (unset) | Enables AI calorie hints |

---

## 11. Seed Data (`cmd/seed/main.go`)

Development helper that populates:
- 2 users (e.g., "Alice", "Bob")
- 10 sample foods across various units
- 14 days of log entries + daily_metrics for each user

Used to quickly spin up a realistic-looking dev environment without manual data entry.

---

## 12. Specifics & Edge Cases

- **Deleted food in logs**: `food_id` is set to NULL via `ON DELETE SET NULL` (or equivalent UPDATE in the delete handler). The snapshot columns (`food_name`, etc.) remain intact, so the UI can still display historical entries correctly.
- **Upsert for metrics**: `INSERT OR REPLACE INTO daily_metrics` — replaces entire row. Partial updates (only weight, leaving steps untouched) require the client to send the full current values, not just the changed field.
- **Recent foods**: Aggregated by `(food_name, food_unit, calories_per_unit)`, ordered by most recent log date, with `last_quantity` as the quantity used in the most recent entry. Useful for quick re-logging of habitual foods.
- **Today summary**: Fetched individually per user on the Home screen — N+1 query pattern (one call per user). Acceptable for household-scale user counts (2–10 people).
- **Chart period calculation**: Done entirely client-side using `date-fns`. The backend just accepts arbitrary `from`/`to` date ranges.
- **SPA fallback**: Any unmatched route in the backend returns `index.html` with `200 OK`, allowing Vue Router to handle client-side navigation (e.g., direct link to `/user/3`).
- **CORS**: Backend sends permissive CORS headers (`Access-Control-Allow-Origin: *`) unconditionally, making it easy to develop locally without proxy configuration.
- **Zero-dependency HTTP**: The backend has exactly one external Go dependency (`modernc.org/sqlite` and its transitive libs). Everything else — JSON encoding, routing, file serving — is stdlib.

---

## 13. Observations & Potential Improvements

| Area | Observation |
|------|-------------|
| Auth | No authentication; adding JWT or session-based auth would be needed if exposed to the internet |
| Metrics upsert | PUT metrics replaces the whole row; a PATCH approach would be friendlier for partial updates |
| N+1 on Home | One `today` request per user could be batched into a single query with GROUP BY |
| AI prompt | Single-line prompt could be more structured (few-shot) for more consistent calorie format |
| SQLite concurrency | Default SQLite WAL mode not explicitly enabled; could cause write contention under concurrent use |
| No pagination | `/api/foods` and `/api/users/{id}/log` return all rows; large datasets could slow down |
| Seed portability | `cmd/seed/main.go` hardcodes the DB path; it should respect `HEALTH_DB` env var |
| Docker volume | DB file lives inside the container by default; a volume mount is required for data persistence |
| Frontend error states | Some API calls lack visible error feedback to the user beyond console errors |
