# Plan: Revamp the User screen into Today / Progress / Log tabs

## Goal

Reshape the per-user screen (`/user/:id`) from one long stacked page into a
**tabbed** experience that visually matches `/home/agent/workspace/mockup.html`
(dark UI, lime accent, mono numerals, tag-tile grid). Three tabs:

- **Today** — today's weekday + date, then a grid of **food-tag tiles**. Tapping a
  tile drills into the recipes carrying that tag, each with an **"Add to log"**
  action (opens the existing recipe drawer pre-selected, with a servings field).
- **Progress** — goal inputs (target calories, target protein) + daily metrics
  (weight, steps) + the existing charts (calories / protein / weight / steps).
- **Log** — **no arrow-key date navigation.** Instead, every date that has logs is
  shown together in **one scrollable pane** (like the mockup), most-recent date
  first, each under a date-group header. Each day's logs keep the **current visual
  format** (recipe group → indented ingredients → per-day total, with delete). A
  **"Create"** button at the top opens the same **Add-to-log** drawer (recent
  recipes + search), logging to today.

We do **not** add "Foods" or "Tags" tabs from the mockup — those already live in the
Ingredient Library (`/library`). We also do a **full cleanup** of functionality that
the revamp makes redundant (see §8).

> Stack: Vue 3 + shadcn-vue (tokenized via CSS vars) + Tailwind. One small **backend
> addition** is needed — an endpoint to fetch a user's full log history across dates
> (§7.1). Everything else reuses existing endpoints.

---

## 1. Target structure

`UserPage.vue` becomes a thin shell: header (back, avatar, name) + a tab bar +
the active tab. Each tab is its own component so the file stays small and each
concern is isolated.

```
UserPage.vue          shell: header + nav tabs + <component :is>
├─ TodayTab.vue       weekday/date header, tag tiles, tag → recipes + Add to log
├─ ProgressTab.vue    goals inputs, today's weight/steps, period selector, charts
└─ LogTab.vue         all-dates scrollable log (grouped by date) + Create button
AddRecipeDrawer.vue   gains optional `initialRecipe` prop (open pre-selected)
ProgressCharts.vue    unchanged (reused by ProgressTab)
```

Routes are unchanged — tabs are in-page state, not routes.

---

## 2. Visual theme (match the mockup)

The mockup is dark with a lime accent, DM Sans body + JetBrains Mono numerals, and
14px radii. Because every shadcn component (Button, Card, Input, Badge, Dialog)
reads the CSS-var tokens, we get ~90% of the restyle by **remapping the `.dark`
tokens** to the mockup palette and turning dark on globally.

### 2.1 Turn dark on + load fonts — `frontend/index.html`

```html
<html lang="en" class="dark">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover" />
    <meta name="theme-color" content="#0e0e0e" />
    <title>Health Tracker</title>
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link
      href="https://fonts.googleapis.com/css2?family=DM+Sans:opsz,wght@9..40,300;9..40,500;9..40,700&family=JetBrains+Mono:wght@400;600&display=swap"
      rel="stylesheet"
    />
  </head>
  ...
```

> If the sandbox firewall blocks `fonts.googleapis.com`, the `font-family`
> fallbacks (`system-ui` / `monospace`) keep the layout intact — the fonts are
> cosmetic. Vendoring them locally is an optional follow-up.

### 2.2 Remap dark tokens to the mockup palette — `frontend/src/style.css`

Replace the `.dark { … }` block's values (hex → HSL triplets) and bump the radius:

```css
.dark {
  --background: 0 0% 5%;          /* #0e0e0e */
  --foreground: 40 13% 89%;       /* #e8e4df */
  --card: 0 0% 9%;               /* #161616 */
  --card-foreground: 40 13% 89%;
  --primary: 72 100% 58%;         /* #d4ff2b lime accent */
  --primary-foreground: 0 0% 5%;  /* dark text on lime */
  --secondary: 0 0% 13%;          /* #222 surface-hover */
  --secondary-foreground: 40 13% 89%;
  --muted: 0 0% 13%;
  --muted-foreground: 40 5% 52%;  /* #8a857e text-dim */
  --accent: 0 0% 13%;
  --accent-foreground: 40 13% 89%;
  --destructive: 0 100% 68%;      /* #ff5c5c */
  --destructive-foreground: 0 0% 5%;
  --border: 0 0% 16%;             /* #2a2a2a */
  --input: 0 0% 16%;
  --ring: 72 100% 58%;
  --radius: 0.875rem;             /* 14px */
}
```

Set the body font to the sans stack (mono is applied per-element with `font-mono`):

```css
  body {
    background: hsl(var(--background));
    color: hsl(var(--foreground));
    font-family: 'DM Sans', system-ui, -apple-system, sans-serif;
    -webkit-tap-highlight-color: transparent;
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
  }
```

### 2.3 Register the fonts in Tailwind — `frontend/tailwind.config.js`

```js
theme: {
  extend: {
    fontFamily: {
      sans: ['"DM Sans"', 'system-ui', 'sans-serif'],
      mono: ['"JetBrains Mono"', 'monospace'],
    },
    colors: { /* …unchanged… */ },
    borderRadius: { /* …unchanged… */ },
  },
},
```

After this, primary buttons render lime-on-dark (matching the mockup's accent
buttons and active nav), cards/inputs/badges go dark, and `font-mono` gives the
JetBrains-Mono numerals. Home and the Library inherit the dark theme for free.

---

## 3. UserPage shell — `frontend/src/views/UserPage.vue`

Replace the whole file. It owns only: user lookup, the active tab, and the header.

```vue
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import Avatar from '@/components/ui/Avatar.vue'
import Button from '@/components/ui/Button.vue'
import TodayTab from '@/components/TodayTab.vue'
import ProgressTab from '@/components/ProgressTab.vue'
import LogTab from '@/components/LogTab.vue'
import { ChevronLeft } from 'lucide-vue-next'

type Tab = 'today' | 'progress' | 'log'

const props = defineProps<{ userId: number }>()
const router = useRouter()
const userStore = useUserStore()

const tab = ref<Tab>('today')
const user = computed(() => userStore.findById(props.userId))

const tabs: { id: Tab; label: string }[] = [
  { id: 'today', label: 'Today' },
  { id: 'progress', label: 'Progress' },
  { id: 'log', label: 'Log' },
]

const activeComponent = computed(() => {
  if (tab.value === 'today') return TodayTab
  if (tab.value === 'progress') return ProgressTab
  return LogTab
})

onMounted(async () => {
  if (userStore.users.length === 0) await userStore.load()
})
</script>

<template>
  <div class="max-w-lg mx-auto flex flex-col h-full">
    <header class="flex items-center gap-3 p-4">
      <Button variant="ghost" size="icon" @click="router.push('/')">
        <ChevronLeft class="h-5 w-5" />
      </Button>
      <Avatar v-if="user" :initials="user.avatar" :seed="user.id" :size="36" />
      <h1 class="text-xl font-bold">{{ user?.name ?? 'User' }}</h1>
    </header>

    <nav class="flex gap-1 mx-4 mb-4 p-1 rounded-xl bg-card border border-border">
      <button
        v-for="t in tabs"
        :key="t.id"
        type="button"
        class="flex-1 h-9 rounded-lg text-sm transition-colors"
        :class="tab === t.id
          ? 'bg-primary text-primary-foreground font-bold'
          : 'text-muted-foreground font-medium hover:bg-secondary'"
        @click="tab = t.id"
      >
        {{ t.label }}
      </button>
    </nav>

    <component :is="activeComponent" :user-id="userId" class="flex-1 overflow-y-auto px-4 pb-12" />
  </div>
</template>
```

---

## 4. Today tab — `frontend/src/components/TodayTab.vue`

Weekday + date, a 2-column tag-tile grid (`listFoodTags`), and a drill-down to the
tag's recipes (`recipesByFoodTag`) where each recipe has **Add to log** (opens the
drawer pre-selected, dated today).

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { format } from 'date-fns'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { FoodTagWithCount, RecipeListItem } from '@/lib/types'
import Button from '@/components/ui/Button.vue'
import AddRecipeDrawer from '@/components/AddRecipeDrawer.vue'

const props = defineProps<{ userId: number }>()

const today = format(new Date(), 'yyyy-MM-dd')
const dayName = format(new Date(), 'EEEE')
const dateLabel = format(new Date(), 'MMMM d, yyyy')

const tags = ref<FoodTagWithCount[]>([])
const loadingTags = ref<boolean>(true)

const selectedTag = ref<FoodTagWithCount | null>(null)
const recipes = ref<RecipeListItem[]>([])
const loadingRecipes = ref<boolean>(false)

const showDrawer = ref<boolean>(false)
const pickedRecipe = ref<RecipeListItem | null>(null)

async function loadTags(): Promise<void> {
  loadingTags.value = true
  try {
    tags.value = await api.listFoodTags()
  } finally {
    loadingTags.value = false
  }
}

async function openTag(t: FoodTagWithCount): Promise<void> {
  selectedTag.value = t
  loadingRecipes.value = true
  try {
    recipes.value = await api.recipesByFoodTag(t.id)
  } finally {
    loadingRecipes.value = false
  }
}

function addToLog(recipe: RecipeListItem): void {
  pickedRecipe.value = recipe
  showDrawer.value = true
}

function onAdded(): void {
  showDrawer.value = false
  pickedRecipe.value = null
}

onMounted(loadTags)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div>
      <div class="font-mono text-xs uppercase tracking-widest text-primary">{{ dayName }}</div>
      <div class="text-3xl font-bold tracking-tight">{{ dateLabel }}</div>
    </div>

    <!-- recipes for the selected tag -->
    <template v-if="selectedTag">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold lowercase">{{ selectedTag.name }}</h2>
        <Button variant="secondary" size="sm" @click="selectedTag = null">← Back</Button>
      </div>
      <div v-if="loadingRecipes" class="flex flex-col gap-2">
        <div v-for="i in 3" :key="i" class="h-20 rounded-xl bg-card animate-pulse" />
      </div>
      <div v-else-if="recipes.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
        No recipes with this tag yet.
      </div>
      <div v-else class="flex flex-col gap-2.5">
        <div
          v-for="r in recipes"
          :key="r.id"
          class="rounded-xl border border-border bg-card p-4 flex items-start justify-between gap-3"
        >
          <div class="min-w-0">
            <div class="font-semibold truncate">{{ r.name }}</div>
            <div class="font-mono text-xs text-muted-foreground mt-1">
              {{ Math.round(r.total_calories) }} cal · {{ formatNumber(r.total_protein, 1) }} g / serving
            </div>
            <div v-if="r.food_tags.length" class="flex flex-wrap gap-1.5 mt-2">
              <span
                v-for="t in r.food_tags"
                :key="t.id"
                class="font-mono text-[10px] tracking-wide text-muted-foreground bg-secondary border border-border rounded px-2 py-1"
              >
                {{ t.name }}
              </span>
            </div>
          </div>
          <Button size="sm" @click="addToLog(r)">Add to log</Button>
        </div>
      </div>
    </template>

    <!-- tag tiles -->
    <template v-else>
      <div v-if="loadingTags" class="grid grid-cols-2 gap-2.5">
        <div v-for="i in 6" :key="i" class="aspect-[1.15] rounded-xl bg-card animate-pulse" />
      </div>
      <div v-else-if="tags.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
        No tags yet — create them in the Library’s “Tags” tab.
      </div>
      <div v-else class="grid grid-cols-2 gap-2.5">
        <button
          v-for="t in tags"
          :key="t.id"
          type="button"
          class="aspect-[1.15] rounded-xl border border-border bg-card p-4 flex flex-col justify-end text-left transition-all hover:-translate-y-0.5 hover:border-primary active:scale-[0.97]"
          @click="openTag(t)"
        >
          <div class="font-semibold">{{ t.name }}</div>
          <div class="font-mono text-[11px] text-muted-foreground mt-0.5">
            {{ t.recipe_count }} recipe{{ t.recipe_count === 1 ? '' : 's' }}
          </div>
        </button>
      </div>
    </template>
  </div>

  <AddRecipeDrawer
    v-if="showDrawer"
    :user-id="userId"
    :date="today"
    :initial-recipe="pickedRecipe"
    @close="showDrawer = false"
    @added="onAdded"
  />
</template>
```

---

## 5. AddRecipeDrawer — open pre-selected

Add an optional `initialRecipe` prop; when present, the drawer opens straight into
the servings state for that recipe (the user still confirms servings → `logRecipe`).

```ts
// AddRecipeDrawer.vue — props + onMounted
import type { RecentItem, RecipeListItem } from '@/lib/types'

const props = defineProps<{
  userId: number
  date: string
  initialRecipe?: RecipeListItem | null
}>()

onMounted(() => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', onViewportChange)
    vv.addEventListener('scroll', onViewportChange)
    onViewportChange()
  }
  if (props.initialRecipe) {
    picked.value = {
      recipe_id: props.initialRecipe.id,
      recipe_name: props.initialRecipe.name,
      total_calories: props.initialRecipe.total_calories,
      total_protein: props.initialRecipe.total_protein,
      scale: '1',
    }
  }
  void loadRecent()
})
```

No other drawer changes — `confirm()` already calls `api.logRecipe` and emits
`added`. The Log tab opens it without `initialRecipe` for free search; Today opens
it with one.

---

## 6. Progress tab — `frontend/src/components/ProgressTab.vue`

Goal inputs + today's weight/steps + period selector + charts. Lifts the metrics /
target / charts logic out of the old `UserPage.vue` (now scoped to **today** rather
than a navigable date).

```vue
<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { format, subDays, subMonths, subYears } from 'date-fns'
import { api } from '@/lib/api'
import { useUserStore } from '@/stores/user'
import type { DailyMetric } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import ProgressCharts from '@/components/ProgressCharts.vue'

type Period = 'w' | 'm' | 'yr'

const props = defineProps<{ userId: number }>()
const userStore = useUserStore()

const today = format(new Date(), 'yyyy-MM-dd')
const user = computed(() => userStore.findById(props.userId))

const weight = ref<string>('')
const steps = ref<string>('')
const targetCalories = ref<string>('')
const targetProtein = ref<string>('')
let savingMetrics = false
let savingTarget = false
let savingTargetProtein = false

const period = ref<Period>('w')
const metricsRange = ref<DailyMetric[]>([])
const loadingCharts = ref(false)

function parseOptionalNumber(v: string): number | null {
  const t = v.trim()
  if (t === '') return null
  const n = Number(t)
  return Number.isFinite(n) ? n : null
}

function syncTargetFromUser(): void {
  const u = user.value
  targetCalories.value = u ? String(u.target_calories) : ''
  targetProtein.value = u ? String(u.target_protein) : ''
}

function dateRangeForPeriod(): { from: string; to: string } {
  const to = new Date()
  const from =
    period.value === 'w' ? subDays(to, 6) : period.value === 'm' ? subMonths(to, 1) : subYears(to, 1)
  return { from: format(from, 'yyyy-MM-dd'), to: format(to, 'yyyy-MM-dd') }
}

async function loadToday(): Promise<void> {
  const range = await api.metricsRange(props.userId, today, today)
  const m = range[0]
  weight.value = m?.weight != null ? String(m.weight) : ''
  steps.value = m?.steps != null ? String(m.steps) : ''
}

async function loadCharts(): Promise<void> {
  loadingCharts.value = true
  try {
    const { from, to } = dateRangeForPeriod()
    metricsRange.value = await api.metricsRange(props.userId, from, to)
  } finally {
    loadingCharts.value = false
  }
}

async function saveMetrics(): Promise<void> {
  if (savingMetrics) return
  savingMetrics = true
  try {
    const w = parseOptionalNumber(weight.value)
    const s = parseOptionalNumber(steps.value)
    await api.saveMetrics(props.userId, {
      date: today,
      weight: w,
      steps: s != null ? Math.round(s) : null,
    })
    await loadCharts()
  } finally {
    savingMetrics = false
  }
}

async function saveTargetCalories(): Promise<void> {
  if (savingTarget) return
  const t = parseOptionalNumber(targetCalories.value)
  if (t == null || t <= 0) { syncTargetFromUser(); return }
  const rounded = Math.round(t)
  if (user.value && user.value.target_calories === rounded) return
  savingTarget = true
  try {
    const updated = await api.updateUser(props.userId, { target_calories: rounded })
    userStore.upsert(updated)
    targetCalories.value = String(updated.target_calories)
  } finally {
    savingTarget = false
  }
}

async function saveTargetProtein(): Promise<void> {
  if (savingTargetProtein) return
  const t = parseOptionalNumber(targetProtein.value)
  if (t == null || t < 0) { syncTargetFromUser(); return }
  const rounded = Math.round(t)
  if (user.value && user.value.target_protein === rounded) return
  savingTargetProtein = true
  try {
    const updated = await api.updateUser(props.userId, { target_protein: rounded })
    userStore.upsert(updated)
    targetProtein.value = String(updated.target_protein)
  } finally {
    savingTargetProtein = false
  }
}

watch(period, loadCharts)
watch(user, syncTargetFromUser, { immediate: true })

onMounted(async () => {
  syncTargetFromUser()
  await loadToday()
  await loadCharts()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <Card>
      <div class="p-4 flex flex-col gap-3">
        <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Goals</div>
        <div class="flex items-center gap-3">
          <label class="w-32 text-sm text-muted-foreground">Target calories</label>
          <Input v-model="targetCalories" type="number" inputmode="numeric" min="1" step="50" @blur="saveTargetCalories" />
          <span class="text-xs text-muted-foreground w-10">kcal</span>
        </div>
        <div class="flex items-center gap-3">
          <label class="w-32 text-sm text-muted-foreground">Target protein</label>
          <Input v-model="targetProtein" type="number" inputmode="numeric" min="0" step="5" @blur="saveTargetProtein" />
          <span class="text-xs text-muted-foreground w-10">g</span>
        </div>
      </div>
    </Card>

    <Card>
      <div class="p-4 flex flex-col gap-3">
        <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Today</div>
        <div class="flex items-center gap-3">
          <label class="w-32 text-sm text-muted-foreground">Weight</label>
          <Input v-model="weight" type="number" inputmode="decimal" min="0" step="0.1" placeholder="0" @blur="saveMetrics" />
          <span class="text-xs text-muted-foreground w-10">kg</span>
        </div>
        <div class="flex items-center gap-3">
          <label class="w-32 text-sm text-muted-foreground">Steps</label>
          <Input v-model="steps" type="number" inputmode="numeric" min="0" step="1" placeholder="0" @blur="saveMetrics" />
          <span class="text-xs text-muted-foreground w-10">steps</span>
        </div>
      </div>
    </Card>

    <div class="flex items-center justify-between">
      <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">Progress</h2>
      <div class="flex gap-1">
        <Button
          v-for="p in (['w', 'm', 'yr'] as Period[])"
          :key="p"
          :variant="period === p ? 'default' : 'outline'"
          size="sm"
          @click="period = p"
        >
          {{ p === 'w' ? 'W' : p === 'm' ? 'M' : 'Yr' }}
        </Button>
      </div>
    </div>
    <div v-if="loadingCharts" class="space-y-3">
      <div v-for="i in 3" :key="i" class="h-48 rounded-lg bg-card animate-pulse" />
    </div>
    <ProgressCharts
      v-else
      :data="metricsRange"
      :target="user?.target_calories ?? 0"
      :protein-target="user?.target_protein ?? 0"
    />
  </div>
</template>
```

---

## 7. Log tab

All dates that have logs, shown together in one scrollable pane (most-recent first),
each day under a date-group header and keeping the **current** per-day visual format
(recipe group → indented ingredients → per-day total, with delete). A **Create**
button at the top opens the Add-to-log drawer. This requires one backend addition to
fetch the full history.

### 7.1 Backend: fetch full log history

The current `GetLog` is single-date (`?date=`). Since the Log tab now shows every
date and nothing else needs the single-date variant, **repurpose** `GET
/api/users/{id}/log` to return the user's full history (no `date` param), grouped
client-side.

`backend/sql/queries/log.sql` — replace `GetLogForDate` with:

```sql
-- name: GetLogHistory :many
SELECT * FROM log_entries
WHERE user_id = ?
ORDER BY date DESC, id;
```

Then `sqlc generate`. `backend/handlers/log.go` — `GetLog` drops the date parsing
and validation:

```go
func (h *Handler) GetLog(w http.ResponseWriter, r *http.Request) {
	userID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	entries, err := h.Q.GetLogHistory(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []queries.LogEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}
```

`backend/main.go` route is unchanged (`GET /api/users/{id}/log`). `DeleteLogEntriesByRecipe`
(used for per-group delete) is untouched — it already takes `date` + `source_recipe_id`.

`frontend/src/lib/api.ts` — `getLog` loses its `date` argument:

```ts
getLog: (userId: number) => request<LogEntry[]>(`${BASE}/users/${userId}/log`),
```

> Returning all history is fine for a personal tracker; if it ever needs bounding,
> add `?from=&to=` later (mirroring `metricsRange`).

### 7.2 LogTab component — `frontend/src/components/LogTab.vue`

Groups the flat history first by **date** (desc), then by **recipe** within each
date (the existing format). Per-day total and per-group delete are preserved. The
legacy `single` branch is dropped (the app is recipe-only — see §8).

```vue
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { format, parseISO } from 'date-fns'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { LogEntry } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import AddRecipeDrawer from '@/components/AddRecipeDrawer.vue'
import { Plus, Trash2 } from 'lucide-vue-next'

interface RecipeGroup {
  recipeId: number
  recipeName: string
  servings: number | null
  entries: LogEntry[]
  totalCalories: number
  totalProtein: number
}
interface DayLog {
  date: string
  displayDate: string
  groups: RecipeGroup[]
  totalCalories: number
  totalProtein: number
}

const props = defineProps<{ userId: number }>()

const today = format(new Date(), 'yyyy-MM-dd')
const entries = ref<LogEntry[]>([])
const loadingLog = ref(false)
const showDrawer = ref(false)

// entries arrive ordered date DESC, id ASC → days desc, recipes first-seen within a day
const days = computed<DayLog[]>(() => {
  const byDate = new Map<string, DayLog>()
  const order: string[] = []
  for (const e of entries.value) {
    if (e.source_recipe_id == null) continue
    let day = byDate.get(e.date)
    if (!day) {
      day = {
        date: e.date,
        displayDate: format(parseISO(e.date), 'MMMM d, yyyy'),
        groups: [],
        totalCalories: 0,
        totalProtein: 0,
      }
      byDate.set(e.date, day)
      order.push(e.date)
    }
    let g = day.groups.find((x) => x.recipeId === e.source_recipe_id)
    if (!g) {
      g = {
        recipeId: e.source_recipe_id,
        recipeName: e.source_recipe_name ?? 'Recipe',
        servings: e.source_recipe_servings,
        entries: [],
        totalCalories: 0,
        totalProtein: 0,
      }
      day.groups.push(g)
    }
    g.entries.push(e)
    g.totalCalories += e.calories
    g.totalProtein += e.protein
    day.totalCalories += e.calories
    day.totalProtein += e.protein
  }
  return order.map((d) => byDate.get(d)!)
})

async function loadLog(): Promise<void> {
  loadingLog.value = true
  try {
    entries.value = await api.getLog(props.userId)
  } finally {
    loadingLog.value = false
  }
}

async function removeRecipeGroup(date: string, recipeId: number): Promise<void> {
  await api.deleteLogRecipeGroup(props.userId, date, recipeId)
  entries.value = entries.value.filter(
    (e) => !(e.date === date && e.source_recipe_id === recipeId),
  )
}

function onAdded(): void {
  showDrawer.value = false
  void loadLog()
}

onMounted(loadLog)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-bold">Logs</h2>
      <Button size="sm" @click="showDrawer = true">
        <Plus class="h-4 w-4" />
        Create
      </Button>
    </div>

    <div v-if="loadingLog" class="flex flex-col gap-3">
      <div v-for="i in 3" :key="i" class="h-32 rounded-xl bg-card animate-pulse" />
    </div>
    <div v-else-if="days.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
      Nothing logged yet — tap “Create” to add a recipe.
    </div>

    <template v-else>
      <div v-for="day in days" :key="day.date" class="flex flex-col gap-2">
        <div class="font-mono text-xs uppercase tracking-wider text-muted-foreground">
          {{ day.displayDate }}
        </div>
        <Card>
          <table class="w-full text-sm">
            <tbody>
              <template v-for="g in day.groups" :key="`r-${day.date}-${g.recipeId}`">
                <tr class="border-b border-border bg-muted/40">
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-2">
                      <span class="font-medium">{{ g.recipeName }}</span>
                      <span class="text-[10px] uppercase tracking-wide text-muted-foreground rounded bg-muted px-1.5 py-0.5">
                        Recipe
                      </span>
                    </div>
                    <div v-if="g.servings != null" class="text-xs text-muted-foreground font-mono">
                      {{ formatNumber(g.servings, g.servings % 1 ? 1 : 0) }}
                      serving{{ g.servings === 1 ? '' : 's' }}
                    </div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">—</td>
                  <td class="text-right px-3 py-2 font-medium font-mono">{{ Math.round(g.totalCalories) }}</td>
                  <td class="text-right px-3 py-2 font-medium font-mono">{{ formatNumber(g.totalProtein, 1) }}</td>
                  <td class="px-1">
                    <Button variant="ghost" size="icon" @click="removeRecipeGroup(day.date, g.recipeId)">
                      <Trash2 class="h-4 w-4" />
                    </Button>
                  </td>
                </tr>
                <tr v-for="e in g.entries" :key="`re-${e.id}`" class="border-b border-border last:border-0">
                  <td class="px-3 py-2 pl-6">
                    <div class="text-muted-foreground">↳ {{ e.ingredient_name }}</div>
                    <div class="text-xs text-muted-foreground">{{ e.ingredient_unit }}</div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">
                    {{ formatNumber(e.quantity, e.quantity % 1 ? 1 : 0) }}
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">{{ Math.round(e.calories) }}</td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">{{ formatNumber(e.protein, 1) }}</td>
                  <td class="px-1" />
                </tr>
              </template>
            </tbody>
          </table>
          <div class="px-3 py-3 border-t border-border flex justify-between font-semibold font-mono">
            <span>Total</span>
            <span>
              <span class="text-primary">{{ formatNumber(Math.round(day.totalCalories)) }} kcal</span>
              <span class="ml-3 text-muted-foreground">{{ formatNumber(day.totalProtein, 1) }} g</span>
            </span>
          </div>
        </Card>
      </div>
    </template>
  </div>

  <AddRecipeDrawer
    v-if="showDrawer"
    :user-id="userId"
    :date="today"
    @close="showDrawer = false"
    @added="onAdded"
  />
</template>
```

---

## 8. Cleanup (remove what's now redundant)

- **Old `UserPage.vue` monolith** — the single stacked layout (Log section + metrics
  Card + Progress section) is fully replaced by the shell + three tab components.
- **Legacy single-entry log rendering** — drop the `LogGroup` `single` variant and
  `removeEntry`. The app is recipe-only (logging always explodes into recipe-tagged
  rows), so the log groups purely by date → recipe. *(Pre-recipes-only "single" rows,
  if any survive in an un-reseeded DB, would no longer render — the dev DB was already
  reseeded; recommend reseeding any other DBs.)*
- **All per-date navigation is removed.** No `prevDay`/`nextDay`/`date` ref/arrow
  controls anywhere. The Log tab shows every date at once; Today is fixed to today;
  Progress metrics are today-scoped — so the old shared `date`-driven metrics editing
  for arbitrary past dates is removed (deliberate simplification matching the mockup).
- **Single-date `GetLogForDate` query is replaced** by `GetLogHistory` (§7.1); the
  `GET /api/users/{id}/log` handler returns full history and `api.getLog` drops its
  `date` argument. The single-date fetch is no longer used anywhere.
- **Unused imports** removed from `UserPage.vue` (date-fns helpers, `Card`, `Input`,
  `ProgressCharts`, chart period type, `Plus`/`Trash2`, etc. now live in the tabs).
- Keep: `AddRecipeDrawer`, `ProgressCharts`, `DonutChart` (still used by Home),
  `Avatar`, all `ui/*`. The only backend change is the log-history endpoint (§7.1).

Nothing else references the removed pieces (verified: `removeEntry`/`single` are
local to `UserPage.vue`; `AddRecipeDrawer` is imported only by the tabs after this;
`GetLogForDate` is used only by the `GetLog` handler).

---

## 9. Implementation order

1. **Backend log history** (§7.1) — `GetLogForDate` → `GetLogHistory` in `log.sql`,
   `sqlc generate`, update `GetLog` handler, `go build ./...` + `go vet ./...`.
2. **Theme** — `index.html` (dark class + fonts), `style.css` (token remap + body
   font), `tailwind.config.js` (font families). Visually verify Home/Library still
   look right in dark.
3. **api.ts** — `getLog(userId)` drops the `date` arg.
4. **AddRecipeDrawer** — add `initialRecipe` prop + pre-select in `onMounted`.
5. **LogTab.vue** — date→recipe grouping over full history + Create button.
6. **ProgressTab.vue** — goals + today metrics + charts.
7. **TodayTab.vue** — date header + tag tiles + drill-down + Add to log.
8. **UserPage.vue** — replace with the shell.
9. `vue-tsc -b` + `npm run build`; manual pass (§10).

Backend first so the frontend builds against the final API. Typecheck after each
component (no `any`/`unknown`).

---

## 10. Manual verification checklist

- [ ] Tapping a user shows **Today** by default; nav switches Today/Progress/Log;
      active tab is lime.
- [ ] Today shows weekday + full date; tag tiles show name + recipe count.
- [ ] Tapping a tile lists that tag's recipes; **Add to log** opens the drawer
      pre-selected with a servings field; confirming logs the recipe and the toast/
      close happens.
- [ ] Progress: editing target calories/protein persists (blur) and updates the
      calorie/protein chart target lines; weight/steps persist for today and appear
      in the charts; W/M/Yr switches ranges.
- [ ] Log: **no date arrows** — every date with logs is shown in one scroll,
      most-recent first, under a date-group header; each day keeps the recipe-group →
      indented-ingredients → per-day-total format; deleting a recipe group removes its
      rows; the top **Create** button opens the drawer (recent recipes) and logs to
      today, after which the new entry appears.
- [ ] `GET /api/users/{id}/log` returns full history (no `date` needed); `go build`/
      `go vet` clean.
- [ ] Dark + lime theme applied app-wide (Home, Library) without broken contrast.
- [ ] `vue-tsc -b` and `npm run build` are clean.
```
