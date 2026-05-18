# Health Tracker — Deep Codebase Research Report

## 1. Project Purpose & Overview

**Health** is a full-stack, multi-user household calorie tracking application. It is built without a cloud dependency, designed to run as a single self-contained Docker container on a home server or local machine. Multiple family members ("users") share one instance; each user independently logs meals, records daily weight/steps, sets calorie targets, defines **recipes** (composite foods like "Mango Shake" made of multiple ingredients), and views progress charts. There is no authentication — the app trusts everyone on the same network.

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
│   │   ├── handlers.go          # Shared helpers; Handler holds *sql.DB + *Queries
│   │   ├── users.go
│   │   ├── foods.go
│   │   ├── log.go               # Single-food logging + recipe-group delete
│   │   ├── recipes.go           # Recipe CRUD + transactional LogRecipe expansion
│   │   ├── metrics.go
│   │   └── ai.go
│   ├── sql/
│   │   ├── schema.sql           # Table definitions
│   │   └── queries/             # Hand-written SQL (input to sqlc)
│   │       ├── users.sql
│   │       ├── foods.sql
│   │       ├── log.sql
│   │       ├── recipes.sql
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
│   │   │   ├── UserPage.vue     # Log table (grouped) + metrics + progress charts
│   │   │   └── FoodLibrary.vue  # Tabbed: Foods + Recipes + AI hint
│   │   └── components/
│   │       ├── DonutChart.vue
│   │       ├── ProgressCharts.vue
│   │       ├── AddFoodDrawer.vue  # Combined food+recipe picker
│   │       ├── RecipeEditor.vue   # New recipe / edit recipe dialog
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
| `source_recipe_id` | INTEGER (nullable) | When entry was created via recipe expansion |
| `source_recipe_name` | TEXT (nullable) | Snapshot of recipe name at log time |

Indexes: `(user_id, date)`, `(source_recipe_id)`

### `recipes` — composite foods (e.g. "Mango Shake")
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `name` | TEXT NOT NULL | |
| `created_at` | TEXT | YYYY-MM-DD |

Recipes carry no `calories_per_unit` — total kcal is derived from the join with `recipe_ingredients` + `foods` at read time.

### `recipe_ingredients` — recipe → food many-to-many with quantity
| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK AUTOINCREMENT | |
| `recipe_id` | INTEGER FK → recipes | `ON DELETE CASCADE` (deleting recipe drops its ingredients) |
| `food_id` | INTEGER FK → foods | `ON DELETE RESTRICT` (cannot delete a food still used by a recipe) |
| `quantity` | REAL NOT NULL CHECK(quantity > 0) | In the food's own unit (g/ml/piece/etc.) |

Index: `(recipe_id)`

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
| GET | `/api/recipes` | `?q=search` | `RecipeListItem[]` | Each row carries derived `total_calories` per serving |
| POST | `/api/recipes` | `{name, ingredients:[{food_id, quantity}]}` | `Recipe` | Atomic create in a transaction |
| GET | `/api/recipes/{id}` | — | `RecipeWithIngredients` | Detail + ingredient rows + total_calories |
| PUT | `/api/recipes/{id}` | `{name, ingredients:[…]}` | `Recipe` | Transactional: update name, clear+reinsert ingredients |
| DELETE | `/api/recipes/{id}` | — | 204 | Cascades to `recipe_ingredients`; past logs unaffected |
| POST | `/api/users/{id}/log/recipe` | `{recipe_id, scale, date}` | `LogEntry[]` | Expands recipe into N log entries inside a transaction |
| DELETE | `/api/users/{id}/log/recipe` | `?date=YYYY-MM-DD&source_recipe_id=N` | 204 | Removes all entries from one recipe-group on a given date |

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
- All `/api/*` routes are registered explicitly. Every other path falls through to the SPA handler which serves `index.html` for client-side routing — **except** paths starting with `/api/`, which return a JSON `{"error":"not found"}` 404 so misrouted API calls never silently get an HTML index page (which would break frontend JSON parsing).
- The pair `DELETE /api/users/{id}/log/recipe` and `DELETE /api/users/{id}/log/{eid}` coexists: Go 1.22 mux prefers the literal "recipe" segment over the `{eid}` wildcard.
- Static files (compiled frontend) are served from a `./frontend/dist/` path relative to the binary (resolved at startup using `os.Executable()`).

### Database (`db/db.go`)
- Single SQLite file; path configurable via `HEALTH_DB` env var (default `health.db`).
- Schema is embedded and run at startup with `CREATE TABLE IF NOT EXISTS` semantics — idempotent, no migration framework needed.
- For column-level migrations (which SQLite cannot express idempotently), an `ensureColumn(table, column, definition)` helper runs **before** the schema. It (a) checks `sqlite_master` for the table existing, (b) reads `PRAGMA table_info` for the column, and (c) only runs `ALTER TABLE … ADD COLUMN` if needed. On fresh DBs the table doesn't exist yet → helper no-ops → `CREATE TABLE` in the schema brings up the columns directly. On legacy DBs the helper backfills the new columns before any dependent `CREATE INDEX` runs. This is how `source_recipe_id` and `source_recipe_name` were added without breaking deployments.
- `modernc.org/sqlite` is a pure-Go port of SQLite; `CGO_ENABLED=0` works, enabling fully static builds and simple Docker cross-compilation.

### SQL Code Generation (`sqlc`)
- All SQL lives in `/backend/sql/queries/*.sql` with `-- name: FunctionName :one/:many/:exec` annotations.
- `sqlc generate` produces type-safe Go structs and method signatures in `/backend/db/queries/`. Generated files are committed to the repo.
- Handlers import the generated `db` package and call methods directly — no string queries in handler code.

### Handlers (`handlers/`)
- Shared helpers in `handlers.go`: `writeJSON`, `writeError`, `readJSON`, `parseID`, `validDate`.
- The `Handler` struct now holds both `*sql.DB` (for opening transactions) **and** `*queries.Queries` (for non-transactional calls). Handlers that need atomicity call `h.DB.BeginTx(…)` and then `h.Q.WithTx(tx)` to get a `Queries` bound to that transaction.
- Each domain (users, foods, log, recipes, metrics, ai) has its own file. Handlers are methods on `*Handler`.
- Transactional handlers:
  - **`CreateRecipe` / `UpdateRecipe`**: insert recipe row + insert all ingredients (or clear-and-reinsert on update) in one tx.
  - **`LogRecipe`**: looks up recipe + ingredients, then inserts N `log_entries` (one per ingredient, scaled, tagged with `source_recipe_id`/`source_recipe_name`) in one tx. A failure mid-way rolls back all of them — no partial meals.
- AI handler constructs a Gemini REST call manually (no SDK); prompt is `"ANSWER IN ONE LINE: how much calorie in {food_name} ?"` and returns the first candidate text. AI hints are intentionally **not** offered for recipes — they're user-authored compositions.

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
- Date picker (defaults today) — fetches log entries and computes a `groups` array that collapses contiguous recipe-sourced entries into a single header row with indented ingredients beneath.
- Each `LogGroup` is either `{ kind: 'single', entry }` or `{ kind: 'recipe', recipeId, recipeName, entries[], totalCalories }`. The table renders single rows as before and recipe groups as a tinted header (with a "Recipe" badge, ingredient count, summed kcal, one trash button) followed by indented ingredient rows.
- Single-row delete hits `DELETE /api/users/{id}/log/{eid}`; recipe-group delete hits `DELETE /api/users/{id}/log/recipe?date=…&source_recipe_id=…` which wipes all three (or N) rows atomically.
- Metrics panel: weight, steps, target calories input fields with debounced PUT.
- "Add Food" button opens `AddFoodDrawer`.
- Period toggle (W / M / Yr) triggers a metrics range fetch, feeding `ProgressCharts`.

**`FoodLibrary.vue`**: Tabbed view with two modes:
- **Foods tab**: search/list/add/delete (existing flow); "✨ AI" button calls `/api/ai/calorie-hint`. Food deletion is blocked by `ON DELETE RESTRICT` if the food is used by any recipe — the UI surfaces the backend error.
- **Recipes tab**: search/list recipes (each row shows kcal/serving), edit (pencil) or delete (trash). Tap "New recipe" to open `RecipeEditor`.

### Components

**`DonutChart.vue`**: Pure SVG. Draws two concentric arcs (consumed vs target) using `stroke-dasharray` / `stroke-dashoffset`. Color-coded: green (under target), amber (near target), red (over target). Accepts `consumed` and `target` props; handles zero-target gracefully.

**`ProgressCharts.vue`**: Wraps Chart.js Line chart (calories vs target) and Bar chart (weight, steps) using vue-chartjs. Receives `DailyMetric[]`; formats dates with `date-fns`. Responsive via Chart.js `maintainAspectRatio: false`.

**`AddFoodDrawer.vue`**: Slide-up overlay. Recent foods (when search is empty) and a **combined library picker** when there's a query — `Promise.all([listFoods, listRecipes])` fans out, results are merged into a `Pickable[]` discriminated union (`{ kind: 'food', food }` | `{ kind: 'recipe', recipe }`), recipes get a "Recipe" badge in the list.
- Picking a **food**: shows quantity input in the food's unit + live kcal preview, POSTs `addLog`.
- Picking a **recipe**: shows a **scale (servings)** input (default 1) + live kcal preview (`scale × total_calories`), POSTs `logRecipe`. Quantity 0.5 means "half a batch"; quantity 2 means "two batches."

**`RecipeEditor.vue`**: Dialog for create/edit. Recipe name field, ingredient list (each row: food name, qty input, unit label, trash). A search box below the list fetches matching foods; tapping one adds it to the recipe at quantity = 1 (de-dup'd by food_id). Live "Total per serving" calculation in a footer panel. Save in create mode → `POST /api/recipes`; edit mode → `PUT /api/recipes/{id}` which transactionally replaces the ingredient list.

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

### Recipes Decompose on Log
A recipe is a **template**, never a row in `log_entries`. When the user logs a "Mango Shake," the server expands the recipe into one `log_entries` row per ingredient. Why this matters:
- Editing or deleting a recipe afterwards never affects past logs (each row already snapshots `food_name`/`food_unit`/`calories_per_unit`).
- All downstream aggregations (today's calories, charts, recent foods) operate on flat ingredient rows and need no recipe-aware logic.
- The same row exists at the food level for both "I had a banana" and "I had a banana via mango shake" — uniform reporting.

### Scale Factor (Not Servings)
The log payload accepts a `scale` float: `1.0` logs the recipe's listed quantities verbatim, `0.5` halves all ingredients, `2.0` doubles. This avoids the recipe needing an explicit "yields N servings" field while still supporting "I drank half the shake."

### Recipe Group Tagging
Each row created via recipe expansion stores `source_recipe_id` + `source_recipe_name` (the latter a snapshot, like food names). This single bit of denormalization powers two UX wins:
1. The log table can collapse a recipe's N ingredient rows into a single visual group with one trash button.
2. Group-delete is a one-shot SQL `DELETE … WHERE source_recipe_id = ?`, atomic and instant.

Because the recipe **name** is snapshotted, renaming or deleting a recipe leaves historical logs displaying the original name correctly.

### Food Deletion Protected by `ON DELETE RESTRICT`
`recipe_ingredients.food_id` uses `ON DELETE RESTRICT`. The user cannot delete a food still referenced by any recipe — the backend returns 500 and the UI surfaces the error. This is intentional: silently setting the ingredient's `food_id` to NULL would make the recipe's calorie calculation undefined. The user must edit the recipe (remove the ingredient) before removing the food.

### No Nested Recipes
A recipe's `food_id` always points to a simple food, never another recipe. Decided explicitly to keep calorie computation a single SQL aggregate and avoid cycle-detection logic. Future feature space, not current.

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

### Creating and Logging a Recipe
1. User goes to `/library`, switches to the **Recipes** tab, taps "New recipe."
2. `RecipeEditor` opens. User types "Mango Shake," searches and adds Mango (200g), Milk (200ml), Sugar (20g). Footer shows live total (~284 kcal/serving).
3. Save → `POST /api/recipes` with `{name, ingredients:[{food_id,quantity}…]}`. Backend opens a transaction, inserts the recipe row, then inserts each ingredient row, commits.
4. Later, on `UserPage`, user taps "Add food," types "mango" — drawer fans out `listFoods` + `listRecipes` in parallel and shows "Mango Shake" (badged "Recipe") alongside food results.
5. User picks the recipe → drawer switches to scale-factor input mode (default 1).
6. User confirms → `POST /api/users/{id}/log/recipe` with `{recipe_id, scale, date}`. Backend:
   - Loads recipe + ingredients
   - Opens a transaction
   - For each ingredient, inserts a `log_entries` row with `quantity = ingredient.quantity × scale`, `calories = cpu × quantity`, and `source_recipe_id`/`source_recipe_name` set
   - Commits and returns the array of inserted entries
7. Frontend refetches the log → `UserPage` groups the three rows under a single "Mango Shake" header.
8. User taps the group's trash → `DELETE /api/users/{id}/log/recipe?date=…&source_recipe_id=…` wipes all three rows in one SQL statement.

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
- **SPA fallback**: Any unmatched non-`/api` route in the backend returns `index.html` with `200 OK`, allowing Vue Router to handle client-side navigation (e.g., direct link to `/user/3`). Unmatched **`/api/*`** paths return a JSON 404 (`{"error":"not found"}`) instead — avoids confusing "Unexpected token '<'" parse errors on the frontend when an API route is missing or misnamed.
- **CORS**: Backend sends permissive CORS headers (`Access-Control-Allow-Origin: *`) unconditionally, making it easy to develop locally without proxy configuration.
- **Zero-dependency HTTP**: The backend has exactly one external Go dependency (`modernc.org/sqlite` and its transitive libs). Everything else — JSON encoding, routing, file serving — is stdlib.
- **Recipe with zero ingredients** is rejected at create time (`at least one ingredient required`) and again at log time (`recipe has no ingredients`) — double-checked because a recipe could in theory have all its ingredients cleared mid-transaction by another caller.
- **Recent foods excludes recipe-sourced rows**: the `GetRecentLoggedFoods` query filters out entries with non-null `source_recipe_id` so the recent-foods picker shows genuinely standalone foods, not the milk/sugar/mango that came from a shake.
- **Route collision avoided**: `DELETE /api/users/{id}/log/recipe` and `DELETE /api/users/{id}/log/{eid}` coexist cleanly because Go 1.22 mux prefers the more specific literal segment.
- **Migration ordering**: `ensureColumn` runs **before** the embedded `schema.sql`. This is necessary because `schema.sql` creates an index on `source_recipe_id` — running it first against a legacy DB (where the column doesn't yet exist) would fail. The helper short-circuits when the table doesn't exist yet (fresh DBs), so the schema's `CREATE TABLE` can bring up the columns natively.

---

## 13. Observations & Potential Improvements

| Area | Observation |
|------|-------------|
| Auth | No authentication; adding JWT or session-based auth would be needed if exposed to the internet |
| Metrics upsert | PUT metrics replaces the whole row; a PATCH approach would be friendlier for partial updates |
| N+1 on Home | One `today` request per user could be batched into a single query with GROUP BY |
| AI prompt | Single-line prompt could be more structured (few-shot) for more consistent calorie format |
| SQLite concurrency | Default SQLite WAL mode not explicitly enabled; could cause write contention under concurrent use |
| No pagination | `/api/foods`, `/api/recipes`, and `/api/users/{id}/log` return all rows; large datasets could slow down |
| Seed portability | `cmd/seed/main.go` hardcodes the DB path; it should respect `HEALTH_DB` env var |
| Docker volume | DB file lives inside the container by default; a volume mount is required for data persistence |
| Frontend error states | Some API calls lack visible error feedback to the user beyond console errors |
| Food-in-use error UX | Deleting a food still used by a recipe returns 500 with a raw SQLite error — could be turned into a 409 with the names of the offending recipes |
| Recipe total caching | `ListRecipes` computes `total_calories` via SQL aggregation on every call; fine for household scale but could be denormalized onto `recipes` as a column updated in the create/update transactions if recipe counts grow |
| Nested recipes | Explicitly excluded; would require a self-referential ingredient table + cycle detection |
