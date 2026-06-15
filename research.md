# Research: `health` — Multi-User Nutrition & Health Tracker

A deep-dive report on how this repository is structured, what it does, and its
notable design choices. Produced by reading the full source tree.

---

## 1. What it is

A small but complete **self-hosted health tracker** for multiple users (intended
for a household/family). Each user tracks:

- **Calories & protein** consumed per day, logged via **recipes** (not individual
  ad-hoc foods — see §6.3).
- **Daily metrics**: body weight (kg) and step count.
- **Goals**: a daily calorie target and a daily protein target.

The data model is built around a reusable **ingredient library** → **recipes**
(ingredient + quantity bundles) → **log entries** (a recipe logged on a date for a
user, expanded into per-ingredient rows). Recipes can be organized with free-form
**food tags** (e.g. "Breakfast", "High Protein"). An optional **AI calorie hint**
feature uses Google Gemini to estimate the calories/protein of a named food.

It is a **single-binary deployment**: the Go backend serves the JSON API *and* the
compiled Vue SPA from one process on `:8080`.

### Tech stack

| Layer    | Technology |
|----------|-----------|
| Frontend | Vue 3 (Composition API, `<script setup>`), TypeScript, Vite, Pinia, Vue Router, Tailwind CSS, Chart.js (via `vue-chartjs`), `date-fns`, `lucide-vue-next` icons |
| Backend  | Go 1.25, stdlib `net/http` (Go 1.22+ method+path routing), `sqlc`-generated query layer |
| Database | SQLite via the **pure-Go** `modernc.org/sqlite` driver (no CGO) |
| AI       | Google Gemini `gemini-2.5-flash` REST API (optional, gated on `GEMINI_API_KEY`) |
| Deploy   | Multi-stage Dockerfile → distroless-ish Alpine image; GitHub Actions builds & pushes to GHCR on every branch push |

---

## 2. Repository layout

```
health/
├── README.md                 # run/build instructions
├── Dockerfile                # 3-stage: build FE → build BE → alpine runtime
├── .github/workflows/build.yml  # CI: build & push image to ghcr.io on push to any branch
├── backend/
│   ├── main.go               # HTTP server, routing, middleware, SPA fallback
│   ├── go.mod / go.sum        # module "health", Go 1.25
│   ├── sqlc.yaml             # sqlc config (sqlite engine)
│   ├── sql/
│   │   ├── schema.sql        # canonical schema (also embedded into the binary)
│   │   ├── queries/*.sql     # hand-written SQL, source for sqlc codegen
│   │   └── migrations/0001_rename_food_to_ingredient.sql  # one-off historical migration
│   ├── db/
│   │   ├── db.go             # DB init, PRAGMAs, ad-hoc column migrations, schema apply
│   │   ├── schema.sql        # embedded copy used by db.go (//go:embed)
│   │   └── queries/*.go      # sqlc-GENERATED code (DO NOT EDIT)
│   ├── handlers/             # one file per resource: users, ingredients, recipes,
│   │   │                       log, food_tags, metrics, ai + handlers.go (shared)
│   └── cmd/seed/main.go      # standalone seeder for demo data
└── frontend/
    ├── index.html, vite.config.ts, tailwind.config.js, tsconfig*.json, postcss.config.js
    └── src/
        ├── main.ts           # app bootstrap (Pinia + Router)
        ├── App.vue           # just <RouterView/>
        ├── router/index.ts   # 3 routes: /, /user/:id, /library
        ├── stores/user.ts    # Pinia store for the user list
        ├── lib/
        │   ├── api.ts        # typed fetch wrapper, one method per endpoint
        │   ├── types.ts      # TS interfaces mirroring backend JSON
        │   └── utils.ts      # cn() + formatNumber()
        ├── views/            # Home, UserPage, IngredientLibrary
        └── components/       # tabs, editors, drawer, charts + ui/ primitives
```

> **Note on schema duplication:** `backend/sql/schema.sql` (the sqlc source) and
> `backend/db/schema.sql` (the `//go:embed`-ed copy applied at runtime) are two
> separate files that must be kept in sync by hand. They are currently identical.

---

## 3. Backend deep-dive

### 3.1 Server & routing (`main.go`)

- Reads DB path from `HEALTH_DB` env (default `health.db`).
- Warns (but does not fail) if `GEMINI_API_KEY` is unset — AI hints simply degrade.
- Uses Go 1.22+ `http.ServeMux` **method + path-pattern routing** with typed path
  params (`{id}`, `{eid}`). All API routes are under `/api/`.
- **Middleware chain:** `cors(logRequest(mux))`.
  - `cors`: wildcard `Access-Control-Allow-Origin: *`, handles `OPTIONS` preflight.
  - `logRequest`: logs `METHOD PATH duration` per request.
- **SPA fallback (`spaHandler`)**: if `../frontend/dist` exists, any non-`/api/`
  path that doesn't map to a real file is served `index.html` (client-side routing).
  Requests under `/api/` that miss a route return a JSON `404`. This is what makes
  the single-binary production mode work.

#### Full API surface

| Method & path | Handler | Purpose |
|---|---|---|
| `GET /api/health` | inline | `{"ok":true}` liveness probe |
| `GET /api/users` | `ListUsers` | list all users |
| `POST /api/users` | `CreateUser` | create user (name, avatar, targets) |
| `PUT /api/users/{id}` | `UpdateUser` | partial update (name/targets) |
| `DELETE /api/users/{id}` | `DeleteUser` | delete user (cascades) |
| `GET /api/users/{id}/today` | `GetTodaySummary` | today's consumed vs target |
| `GET /api/ingredients` | `ListIngredients` | list/search ingredients (`?q=`) |
| `POST /api/ingredients` | `CreateIngredient` | create ingredient |
| `PUT /api/ingredients/{id}` | `UpdateIngredient` | update nutrition + **restamp logs** |
| `DELETE /api/ingredients/{id}` | `DeleteIngredient` | delete ingredient |
| `GET /api/users/{id}/log` | `GetLog` | full log history |
| `POST /api/users/{id}/log/recipe` | `LogRecipe` | log a recipe → many log rows |
| `DELETE /api/users/{id}/log/recipe` | `DeleteLogEntriesByRecipe` | delete a recipe group (`?date=&source_recipe_id=`) |
| `DELETE /api/users/{id}/log/{eid}` | `DeleteLogEntry` | delete single log row |
| `GET /api/users/{id}/recent` | `GetRecentRecipes` | recently logged recipes (7-day window) |
| `GET /api/recipes` | `ListRecipes` | list/search recipes w/ totals + tags |
| `POST /api/recipes` | `CreateRecipe` | create recipe (tx) |
| `GET /api/recipes/{id}` | `GetRecipe` | recipe detail w/ ingredients + tags |
| `PUT /api/recipes/{id}` | `UpdateRecipe` | replace recipe (tx) |
| `DELETE /api/recipes/{id}` | `DeleteRecipe` | delete recipe |
| `GET /api/food-tags` | `ListFoodTags` | tags w/ recipe counts |
| `POST /api/food-tags` | `CreateFoodTag` | create tag (unique, case-insensitive) |
| `DELETE /api/food-tags/{id}` | `DeleteFoodTag` | delete tag (cascades from recipes) |
| `GET /api/food-tags/{id}/recipes` | `GetRecipesByFoodTag` | recipes carrying a tag |
| `GET /api/users/{id}/metrics` | `GetMetrics` | weight/steps + consumed nutrition over range |
| `PUT /api/users/{id}/metrics` | `UpsertMetrics` | upsert weight/steps for a date |
| `POST /api/ai/calorie-hint` | `CalorieHint` | Gemini calorie/protein estimate |

There is **no authentication or authorization** anywhere — any client can act as
any user by ID. This is a trusted-LAN / personal-deployment app.

### 3.2 Database layer (`db/db.go`)

`db.Init(path)`:
1. Opens SQLite via `modernc.org/sqlite` (pure Go, CGO-free → trivial cross-compile,
   matches the `CGO_ENABLED=0` build in the Dockerfile).
2. `PRAGMA foreign_keys = ON` — FK enforcement is **on** (important: cascades and
   `RESTRICT` rules below actually fire).
3. Runs a series of **idempotent ad-hoc migrations** *before* applying the schema:
   - `ensureColumn(...)` — adds a column via `ALTER TABLE ... ADD COLUMN` only if the
     table exists and the column is missing (introspects `PRAGMA table_info`). Used to
     evolve `log_entries` (source recipe columns, protein columns), `users` (targets),
     and `ingredients` (protein).
   - `dropColumnIfExists(...)` — drops `daily_metrics.target_calories` if present
     (a target that was relocated onto `users`).
4. Splits the embedded `schema.sql` on `;` and executes each `CREATE TABLE IF NOT
   EXISTS` / `CREATE INDEX IF NOT EXISTS` statement. So schema creation is idempotent
   and runs on every boot.

This hybrid approach (idempotent `CREATE IF NOT EXISTS` + targeted `ensureColumn`/
`dropColumnIfExists`) is a lightweight homegrown migration system — there is **no
migration framework or version table**. The `sql/migrations/0001_*.sql` file is a
*manual* one-off (run via the `sqlite3` CLI) documenting the historical rename of
"food" → "ingredient"; it is not executed by the app.

### 3.3 Schema (`sql/schema.sql`)

Eight tables:

- **`users`** — `name`, `avatar` (initials), `target_calories` (default 2000),
  `target_protein` (default 0), `created_at`.
- **`ingredients`** — `name`, `unit` (default `'g'`), `calories_per_unit` (REAL),
  `protein_per_unit` (REAL). The reusable nutrition library.
- **`log_entries`** — the central denormalized table. Each row is **one ingredient
  consumed by one user on one date**. It snapshots the ingredient's name, unit,
  calories/protein per unit at log time, plus `quantity`, computed `calories` &
  `protein`, and **source-recipe provenance** (`source_recipe_id`,
  `source_recipe_name`, `source_recipe_servings`). `ingredient_id` is
  `ON DELETE SET NULL` so deleting a library ingredient preserves history.
- **`recipes`** — `name`, `created_at`.
- **`recipe_ingredients`** — junction: `recipe_id` (`ON DELETE CASCADE`),
  `ingredient_id` (`ON DELETE RESTRICT` — can't delete an ingredient still used by a
  recipe), `quantity` (`CHECK(quantity > 0)`).
- **`food_tags`** — `name` is `COLLATE NOCASE UNIQUE` (case-insensitive uniqueness).
- **`recipe_food_tags`** — many-to-many junction, composite PK, both FKs cascade.
- **`daily_metrics`** — per-user-per-day `weight`/`steps`, `UNIQUE(user_id, date)`
  enabling upsert.

Indexes cover the hot lookups: `log_entries(user_id, date)`,
`log_entries(source_recipe_id)`, `daily_metrics(user_id, date)`,
`recipe_ingredients(recipe_id)`, `recipe_food_tags(food_tag_id)`.

**Key design idea — log entries are immutable snapshots.** A log row stores the
nutrition values as they were, independent of the live ingredient. This means recipe
or ingredient edits don't silently rewrite history… *except* by deliberate design in
one case (see §3.5).

### 3.4 Query layer (`sqlc`)

`sqlc.yaml` configures the SQLite engine, reads `sql/queries/*.sql` + `sql/schema.sql`,
and generates `db/queries/*.go` with:
- `emit_json_tags: true` → generated structs carry `json:"snake_case"` tags, so
  handlers can serialize sqlc rows directly to the API (the TS `types.ts` mirrors
  these exactly).
- `emit_pointers_for_null_types: true` → nullable columns become Go pointers
  (`*float64`, `*int64`, `*string`), which serialize to JSON `null`.

`Queries` carries a `DBTX` interface and a `WithTx(tx)` helper, letting handlers run
the same generated methods inside a transaction (used by recipe create/update,
recipe logging, and ingredient update).

Notable queries:
- **`GetRecentLoggedRecipes`** (`log.sql`) — a non-trivial query: a subquery finds the
  `MAX(log_entries.id)` per `source_recipe_id` within a date floor, joins back to get
  the last servings, and re-aggregates the recipe's current ingredient totals.
- **`GetTodaySummary`** (`metrics.sql`) — correlated subqueries sum today's
  (`date('now')`) calories & protein and join the user's targets in one row.
- **`SumNutritionByDateRange`** — per-day calorie/protein sums, joined client-side in
  Go against the metrics range.
- **`RestampLogEntriesForIngredient`** (`ingredients.sql`) — see §3.5.
- Search queries use `name LIKE '%' || ? || '%'` for substring search.

### 3.5 Handler logic & business rules

Shared helpers (`handlers.go`): `writeJSON`/`writeError` (JSON envelope
`{"error": "..."}`), `readJSON`, `parseID` (rejects ≤0 / non-numeric), and
`validDate` (strict `^\d{4}-\d{2}-\d{2}$` regex — note: **format-only**, doesn't
validate that e.g. month ≤ 12).

Notable behaviors:

- **Partial user update** (`UpdateUser`): loads current row, applies only the
  non-nil pointer fields from the body, re-validates. (Avatar is not updatable here.)
- **Avatar derivation** (`CreateUser`): if no avatar provided, uses the first two
  uppercased letters of the name.
- **Ingredient edit restamps history** (`UpdateIngredient`): in a single transaction
  it updates the ingredient's nutrition **and** runs `RestampLogEntriesForIngredient`,
  which rewrites `calories_per_unit/protein_per_unit` *and recomputes
  `calories`/`protein`* on **every existing log entry** referencing that ingredient.
  This is a deliberate exception to log immutability — correcting an ingredient's
  nutrition retroactively fixes all past logs. The UI warns the user about this.
  (Only nutrition is editable post-creation; name and unit are immutable in the UI.)
- **Recipe create/update are transactional** and "replace-all": update clears and
  re-inserts all recipe_ingredients and recipe_food_tags. Tag application
  (`applyRecipeFoodTags`) validates each tag exists (400 if not) and uses
  `INSERT OR IGNORE` to be idempotent.
- **Logging a recipe** (`LogRecipe`): validates recipe exists & has ingredients, then
  within a transaction expands the recipe into N log rows — each ingredient's
  `quantity * servings`, with calories/protein computed from the **current** ingredient
  values and stamped with recipe provenance. Returns the created rows (201).
- **Recent recipes** (`GetRecentRecipes`): queries a 7-day window
  (`recentWindowDays = 7`), sorts in Go by `maxID` desc (most-recently-logged first),
  caps at `recentItemsCap = 20`. The `maxID` is an unexported struct field excluded
  from JSON.
- **Food tag list** carries a `recipe_count` (LEFT JOIN + COUNT) for the UI.
- **Tag→recipe and recipe lists** attach each recipe's full tag set by building a
  `map[recipeID][]FoodTag` from `ListAllRecipeFoodTags` (a single query, joined in Go
  rather than N+1).
- **Metrics** (`GetMetrics`): merges `daily_metrics` rows with per-day consumed
  nutrition (two queries, joined via maps keyed by date) into a `metricsResponse`.
  `UpsertMetrics` relies on the `UNIQUE(user_id, date)` + `ON CONFLICT DO UPDATE`.

### 3.6 AI calorie hint (`ai.go`)

`POST /api/ai/calorie-hint` with `{"name": "..."}`:
- Returns `503` if `GEMINI_API_KEY` unset.
- Builds a one-line prompt: `"ANSWER IN ONE LINE: how much calories and protein (g)
  in {name} ?"` and POSTs to the Gemini `gemini-2.5-flash:generateContent` endpoint
  with the key as a query param.
- Parses the first candidate's first text part and returns `{"hint": "..."}`.
- Returns `502` on network failure or unexpected response shape. Pure text hint —
  the result is **advisory only** and is **not parsed into structured numbers**; the
  user still types the values manually.

### 3.7 Seeder (`cmd/seed/main.go`)

A standalone `main` (run via `go run ./cmd/seed`) that opens the same DB and inserts:
- 2 users (Mohit, Sara) with targets.
- 10 ingredients with realistic per-unit calories/protein.
- 4 recipes wired to ingredients and tagged (Breakfast/Lunch/Vegetarian/High Protein),
  ensuring tags exist on demand.
- ~14 days of `daily_metrics` (weight trending slightly down, random steps) and
  2–3 logged meals/day per user, using a **seeded RNG (`rand.NewSource(42)`)** so
  output is deterministic.

---

## 4. Frontend deep-dive

### 4.1 Bootstrap & routing

`main.ts` mounts `App.vue` (just `<RouterView/>`) with Pinia + Router. Three routes
(all lazy-loaded):
- `/` → `Home.vue` — user picker / dashboard.
- `/user/:id` → `UserPage.vue` — per-user screen (prop `userId` coerced to Number).
- `/library` → `IngredientLibrary.vue` — ingredient/recipe/tag management.

`vite.config.ts` sets the `@` → `src` alias and proxies `/api` → `http://localhost:8080`
in dev (so the SPA and API share an origin during development).

### 4.2 State, API & types

- **`stores/user.ts`** (Pinia, setup style): holds the `users` list with `load`, `add`,
  `upsert`, `findById`. It is the only global store; everything else is component-local.
- **`lib/api.ts`**: a typed `fetch` wrapper. `request<T>()` sets JSON headers, throws
  an `Error` carrying the server's `{error}` message on non-2xx, and returns `undefined`
  for `204`. Exposes one method per endpoint — a clean, fully-typed client.
- **`lib/types.ts`**: TS interfaces that mirror the Go JSON exactly (`User`,
  `Ingredient`, `LogEntry`, `RecipeWithIngredients`, `DailyMetric`, `FoodTagWithCount`,
  payload types, etc.). Nullable backend fields are `T | null`.
- **`lib/utils.ts`**: `cn()` (class-name joiner) and `formatNumber()` (locale-aware,
  fixed digits).

### 4.3 Views

- **`Home.vue`** — lists users as cards, each showing a `DonutChart` of today's
  consumed/target calories and a one-line summary (fetched per user via
  `Promise.all` over `api.todaySummary`). Has an "Add user" dialog with client-side
  validation (positive calorie target, non-negative protein). Links to the Food Library.
- **`UserPage.vue`** — a header (back button, avatar, name) plus a **3-tab switcher**
  (`Today` / `Progress` / `Log`) rendered via `<component :is>`; passes `userId` down.
  Loads the user list if empty so the page is deep-linkable.
- **`IngredientLibrary.vue`** — three sub-tabs (`Ingredients` / `Recipes` / `Tags`)
  with debounced (200 ms) search, CRUD via the editor dialogs, and a tag-drill-down
  view (select a tag → list its recipes). Uses `confirm()` for deletes and surfaces
  backend errors (e.g. can't delete an ingredient used by a recipe) via `alert()`.

### 4.4 Components

**Feature tabs:**
- **`TodayTab.vue`** — quick logging surface. Shows today's date, food tags as a
  2-col grid; selecting a tag lists its recipes; "add" opens `AddRecipeDrawer`
  pre-seeded with the chosen recipe. Uses `date-fns` for date display.
- **`LogTab.vue`** — chronological history grouped **by date, then by recipe**
  (`source_recipe_id`), rendered as tables with per-group and per-day calorie/protein
  totals. Deletes a whole recipe group via `deleteLogRecipeGroup`. Only shows
  recipe-sourced entries (filters `source_recipe_id == null`).
- **`ProgressTab.vue`** — editable **goals** (calorie/protein targets, persisted via
  `updateUser` + store upsert) and **today's metrics** (weight/steps, auto-saved on
  blur via `saveMetrics`), plus a `w`/`m`/`yr` period selector that drives
  `metricsRange` and renders `ProgressCharts`. Validates/parses optional numbers and
  reverts invalid input to stored values; guards against duplicate saves.
- **`ProgressCharts.vue`** — four Chart.js charts (Calories & Protein line charts with
  dashed target reference lines, Weight line, Steps bar). Resolves theme colors at
  runtime from CSS variables via `getComputedStyle` (so charts match light/dark theme).
  Shows "Not enough data" when empty.

**Editors / drawer:**
- **`RecipeEditor.vue`** — create/edit dialog: name, toggleable tag pills, an
  ingredient list with debounced ingredient search + add (dedupes), per-serving
  total calories/protein, validation, and `createRecipe`/`updateRecipe`. Edit mode
  loads via `getRecipe`.
- **`AddRecipeDrawer.vue`** — bottom-sheet for logging. Shows "Recent" recipes
  (seeded with last servings) when query empty, search results otherwise; lets you
  pick servings (decimal) with live calorie/protein preview, then `logRecipe`.
  Mobile-aware: tracks `window.visualViewport` to stay above the on-screen keyboard,
  locks body scroll, closes on Escape. Teleported to `<body>`.
- **`IngredientEditor.vue`** — create/edit ingredient. Create mode offers a "✨ AI"
  button calling `calorieHint` for a text estimate; name/unit immutable in edit mode;
  warns that nutrition edits restamp past logs (matching backend §3.5).
- **`FoodTagEditor.vue`** — minimal single-field "new tag" dialog.

**Visuals:**
- **`DonutChart.vue`** — SVG ring of consumed/target %, color-coded blue → amber
  (≥80%) → red (≥100%); shows "—" when no target. Pure computed geometry, no library.

**UI primitives (`components/ui/`)** — a hand-rolled **shadcn-vue-style** kit:
`Avatar` (deterministic color from a hash of seed/initials), `Badge`, `Button`
(variants/sizes), `Card`, `Dialog` (teleported modal, Escape + backdrop close, body
scroll lock), `Input` (v-model, mobile `inputmode`). All styled with Tailwind +
the CSS-variable theme.

### 4.5 Styling

`style.css` defines the shadcn-style **HSL CSS-variable palette** for `:root` (light)
and `.dark` (dark) — `--background`, `--foreground`, `--primary`, `--card`,
`--muted`, `--destructive`, `--border`, `--ring`, plus chart colors
(`--chart-blue/amber/green/violet`) and `--radius`. `tailwind.config.js` maps these
variables into Tailwind color tokens and sets `darkMode: 'class'`. Mobile niceties:
safe-area insets on `body`, removed number-input spinners, disabled tap highlight.
(No dark-mode toggle is wired up in the app — the `.dark` variables exist but nothing
sets the class.)

---

## 5. Build, deploy & CI

- **Dev:** backend `go run .` (`:8080`); frontend `npm run dev` (`:5173`, proxies
  `/api`). `npm run build` runs `vue-tsc -b` (type-check) then `vite build` → `dist/`.
- **Production single binary:** build the FE, `go build -o health`, run `./health` —
  it serves API + the `dist/` SPA from `../frontend/dist`.
- **Dockerfile:** 3 stages — (1) `node:22-alpine` builds the FE, (2) `golang:1.25-alpine`
  builds a static (`CGO_ENABLED=0`) server binary, (3) `alpine:3.21` runtime with
  `ca-certificates`/`tzdata`, copying the server + `dist/`. Final layout mirrors the
  repo (`/app/backend/server`, `/app/frontend/dist`) so the relative `../frontend/dist`
  path resolves. Exposes `8080`.
- **CI (`.github/workflows/build.yml`):** on push to **any** branch, logs into GHCR
  and builds+pushes `ghcr.io/<repo>:<branch>` with GH Actions layer caching. No test
  or lint stage.

---

## 6. Observations, specificities & gotchas

### 6.1 Naming churn: "food" → "ingredient"
The codebase was refactored from "food" to "ingredient" (commit `64faf68`, plus
`migrations/0001`). The rename is **incomplete at the surface level**: the route
namespace and UI still say "Food Library" / "food-tags", the table is `food_tags`,
and SQL aliases the ingredients table as `f` in several queries. The *data* concept
is now "ingredient", but "food" survives in tags and labels. Worth knowing when
grepping.

### 6.2 The `add-exercises` branch has no exercises yet
The current branch is `add-exercises`, but it contains **no exercise/workout code** —
its diff vs `main` is a **UI revamp into tabs** plus deletion of the old `features/`
planning docs and top-level `research.md`/`plan.md` (commits `083c0d4 revamp ui into
tabs`, `a6d5bf4 remove ai generated pages`). So "add-exercises" is the in-progress
working branch; exercise tracking is presumably the *next* feature, not yet started.
(This report re-creates `research.md`, which was previously deleted.)

### 6.3 Logging is recipe-only
You cannot log a bare ingredient ad-hoc; you log a **recipe** (with servings), which
fans out into per-ingredient log rows. A single ingredient is logged by first making
a one-ingredient recipe. `LogTab` even filters out any entry lacking a
`source_recipe_id`. This is a deliberate product decision (commit `c148e18 interm:
only log recipes`).

### 6.4 Two sources of truth for the schema
`backend/sql/schema.sql` (sqlc) and `backend/db/schema.sql` (embedded, runtime) are
separate files kept in sync manually. Diverging them would make codegen and runtime
disagree.

### 6.5 Log immutability vs. ingredient restamping
Log rows are otherwise immutable snapshots, but editing an ingredient's nutrition
**retroactively rewrites every past log entry** for it (`RestampLogEntriesForIngredient`).
This is intentional and surfaced to the user, but is a sharp edge: a correction today
changes historical daily totals and charts.

### 6.6 Security / multi-tenancy
No auth, no per-user access control, wildcard CORS. Any caller can read/modify any
user's data by ID. Appropriate only for a private/LAN deployment.

### 6.7 Date handling
The backend trusts the **client-supplied date** for logging and metrics (validated by
regex only, format `YYYY-MM-DD`), while "today" summaries use SQLite `date('now')`
(UTC). The frontend computes "today" with `date-fns` in local time. Around midnight /
across timezones, the client's "today" and the server's `date('now')` can disagree —
a logged entry's date (client-derived) may not match the day the `today` summary
counts. The date regex is also format-only (e.g. `2026-13-40` passes).

### 6.8 No automated tests
There are no Go `*_test.go` files and no frontend test setup; CI only builds an image.
Verified during research: `go vet ./...` is clean and `go build` succeeds.

---

## 7. Quick mental model

> **Ingredients** (reusable nutrition facts) are composed into **recipes** (ingredient
> + quantity, optionally tagged). A user **logs a recipe** for a date with a serving
> count; the server explodes it into immutable per-ingredient **log entries** stamped
> with the recipe's identity. **Today** view sums today's entries against the user's
> **targets**; **Progress** view charts weight/steps/calories/protein over time from
> **daily_metrics** + summed log entries; **Log** view shows the grouped history. The
> Go binary serves both the API and the Vue SPA; SQLite is the only datastore.
```
