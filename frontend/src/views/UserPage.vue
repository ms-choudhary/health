<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  format,
  addDays,
  isToday as dateFnsIsToday,
  subDays,
  subMonths,
  subYears,
  parseISO,
} from 'date-fns'
import { api } from '@/lib/api'
import { useUserStore } from '@/stores/user'
import type { LogEntry, DailyMetric } from '@/lib/types'

type LogGroup =
  | { kind: 'single'; entry: LogEntry }
  | {
      kind: 'recipe'
      recipeId: number
      recipeName: string
      servings: number | null
      entries: LogEntry[]
      totalCalories: number
      totalProtein: number
    }
import { formatNumber } from '@/lib/utils'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Avatar from '@/components/ui/Avatar.vue'
import AddFoodDrawer from '@/components/AddFoodDrawer.vue'
import ProgressCharts from '@/components/ProgressCharts.vue'
import { ChevronLeft, ChevronRight, Plus, Trash2 } from 'lucide-vue-next'

type Period = 'w' | 'm' | 'yr'

const props = defineProps<{ userId: number }>()
const router = useRouter()
const userStore = useUserStore()

const today = format(new Date(), 'yyyy-MM-dd')
const date = ref<string>(today)
const entries = ref<LogEntry[]>([])
const loadingLog = ref(false)

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

const showDrawer = ref(false)

const isToday = computed(() => dateFnsIsToday(parseISO(date.value)))
const displayDate = computed(() => format(parseISO(date.value), 'd MMM yyyy'))
const totalCalories = computed(() =>
  Math.round(entries.value.reduce((s, e) => s + e.calories, 0)),
)
const totalProtein = computed(() =>
  entries.value.reduce((s, e) => s + e.protein, 0),
)
const user = computed(() => userStore.findById(props.userId))

const groups = computed<LogGroup[]>(() => {
  const out: LogGroup[] = []
  const recipeMap = new Map<number, LogEntry[]>()
  const order: Array<{ kind: 'single'; entry: LogEntry } | { kind: 'recipe'; recipeId: number }> = []
  for (const e of entries.value) {
    if (e.source_recipe_id != null) {
      const arr = recipeMap.get(e.source_recipe_id)
      if (arr) {
        arr.push(e)
      } else {
        recipeMap.set(e.source_recipe_id, [e])
        order.push({ kind: 'recipe', recipeId: e.source_recipe_id })
      }
    } else {
      order.push({ kind: 'single', entry: e })
    }
  }
  for (const item of order) {
    if (item.kind === 'single') {
      out.push({ kind: 'single', entry: item.entry })
      continue
    }
    const arr = recipeMap.get(item.recipeId)
    if (!arr || arr.length === 0) continue
    out.push({
      kind: 'recipe',
      recipeId: item.recipeId,
      recipeName: arr[0].source_recipe_name ?? 'Recipe',
      servings: arr[0].source_recipe_servings,
      entries: arr,
      totalCalories: arr.reduce((s, e) => s + e.calories, 0),
      totalProtein: arr.reduce((s, e) => s + e.protein, 0),
    })
  }
  return out
})

function prevDay() {
  date.value = format(addDays(parseISO(date.value), -1), 'yyyy-MM-dd')
}
function nextDay() {
  if (isToday.value) return
  date.value = format(addDays(parseISO(date.value), 1), 'yyyy-MM-dd')
}

async function loadLog() {
  loadingLog.value = true
  try {
    entries.value = await api.getLog(props.userId, date.value)
    const range = await api.metricsRange(props.userId, date.value, date.value)
    const m = range[0]
    weight.value = m?.weight != null ? String(m.weight) : ''
    steps.value = m?.steps != null ? String(m.steps) : ''
  } finally {
    loadingLog.value = false
  }
}

function syncTargetFromUser(): void {
  const u = user.value
  targetCalories.value = u ? String(u.target_calories) : ''
  targetProtein.value = u ? String(u.target_protein) : ''
}

function dateRangeForPeriod(): { from: string; to: string } {
  const to = new Date()
  const from =
    period.value === 'w'
      ? subDays(to, 6)
      : period.value === 'm'
        ? subMonths(to, 1)
        : subYears(to, 1)
  return { from: format(from, 'yyyy-MM-dd'), to: format(to, 'yyyy-MM-dd') }
}

async function loadCharts() {
  loadingCharts.value = true
  try {
    const { from, to } = dateRangeForPeriod()
    metricsRange.value = await api.metricsRange(props.userId, from, to)
  } finally {
    loadingCharts.value = false
  }
}

function parseOptionalNumber(v: string): number | null {
  const trimmed = v.trim()
  if (trimmed === '') return null
  const n = Number(trimmed)
  return Number.isFinite(n) ? n : null
}

async function saveMetrics() {
  if (savingMetrics) return
  savingMetrics = true
  try {
    const w = parseOptionalNumber(weight.value)
    const s = parseOptionalNumber(steps.value)
    await api.saveMetrics(props.userId, {
      date: date.value,
      weight: w,
      steps: s != null ? Math.round(s) : null,
    })
    await loadCharts()
  } finally {
    savingMetrics = false
  }
}

async function saveTargetCalories() {
  if (savingTarget) return
  const t = parseOptionalNumber(targetCalories.value)
  if (t == null || t <= 0) {
    syncTargetFromUser()
    return
  }
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

async function saveTargetProtein() {
  if (savingTargetProtein) return
  const t = parseOptionalNumber(targetProtein.value)
  if (t == null || t < 0) {
    syncTargetFromUser()
    return
  }
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

async function removeEntry(id: number) {
  await api.deleteLog(props.userId, id)
  entries.value = entries.value.filter((e) => e.id !== id)
}

async function removeRecipeGroup(recipeId: number) {
  await api.deleteLogRecipeGroup(props.userId, date.value, recipeId)
  entries.value = entries.value.filter((e) => e.source_recipe_id !== recipeId)
}

async function onFoodAdded() {
  showDrawer.value = false
  await loadLog()
}

watch(date, loadLog)
watch(period, loadCharts)
watch(user, syncTargetFromUser, { immediate: true })

onMounted(async () => {
  if (userStore.users.length === 0) await userStore.load()
  syncTargetFromUser()
  await loadLog()
  await loadCharts()
})
</script>

<template>
  <div class="max-w-lg mx-auto p-4 sm:p-6 pb-12 flex flex-col gap-6">
    <header class="flex items-center gap-3">
      <Button variant="ghost" size="icon" @click="router.push('/')">
        <ChevronLeft class="h-5 w-5" />
      </Button>
      <Avatar v-if="user" :initials="user.avatar" :seed="user.id" :size="36" />
      <h1 class="text-xl font-bold">{{ user?.name ?? 'User' }}</h1>
    </header>

    <section class="flex flex-col gap-3">
      <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
        Log
      </h2>

      <div class="flex items-center justify-between">
        <Button variant="ghost" size="icon" @click="prevDay">
          <ChevronLeft class="h-5 w-5" />
        </Button>
        <span class="font-semibold">{{ displayDate }}</span>
        <Button variant="ghost" size="icon" :disabled="isToday" @click="nextDay">
          <ChevronRight class="h-5 w-5" />
        </Button>
      </div>

      <Card>
        <div v-if="loadingLog" class="p-4 space-y-2">
          <div v-for="i in 3" :key="i" class="h-8 rounded bg-muted animate-pulse" />
        </div>
        <div v-else-if="entries.length === 0" class="p-6 text-center text-muted-foreground text-sm">
          No food logged yet.
        </div>
        <table v-else class="w-full text-sm">
          <thead class="text-xs text-muted-foreground">
            <tr class="border-b border-border">
              <th class="text-left font-medium px-3 py-2">Food</th>
              <th class="text-right font-medium px-3 py-2 w-16">Qty</th>
              <th class="text-right font-medium px-3 py-2 w-16">Cal</th>
              <th class="text-right font-medium px-3 py-2 w-16">Prot</th>
              <th class="w-9" />
            </tr>
          </thead>
          <tbody>
            <template
              v-for="g in groups"
              :key="g.kind === 'single' ? `s-${g.entry.id}` : `r-${g.recipeId}`"
            >
              <tr v-if="g.kind === 'single'" class="border-b border-border last:border-0">
                <td class="px-3 py-2">
                  <div>{{ g.entry.food_name }}</div>
                  <div class="text-xs text-muted-foreground">{{ g.entry.food_unit }}</div>
                </td>
                <td class="text-right px-3 py-2">
                  {{ formatNumber(g.entry.quantity, g.entry.quantity % 1 ? 1 : 0) }}
                </td>
                <td class="text-right px-3 py-2">{{ Math.round(g.entry.calories) }}</td>
                <td class="text-right px-3 py-2">{{ formatNumber(g.entry.protein, 1) }}</td>
                <td class="px-1">
                  <Button variant="ghost" size="icon" @click="removeEntry(g.entry.id)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                </td>
              </tr>
              <template v-else>
                <tr class="border-b border-border bg-muted/40">
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-2">
                      <span class="font-medium">{{ g.recipeName }}</span>
                      <span class="text-[10px] uppercase tracking-wide text-muted-foreground rounded bg-muted px-1.5 py-0.5">
                        Recipe
                      </span>
                    </div>
                    <div class="text-xs text-muted-foreground">
                      <template v-if="g.servings != null">
                        {{ formatNumber(g.servings, g.servings % 1 ? 1 : 0) }}
                        serving{{ g.servings === 1 ? '' : 's' }}
                      </template>
                      <template v-else>
                        {{ g.entries.length }} ingredient{{ g.entries.length === 1 ? '' : 's' }}
                      </template>
                    </div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">—</td>
                  <td class="text-right px-3 py-2 font-medium">
                    {{ Math.round(g.totalCalories) }}
                  </td>
                  <td class="text-right px-3 py-2 font-medium">
                    {{ formatNumber(g.totalProtein, 1) }}
                  </td>
                  <td class="px-1">
                    <Button variant="ghost" size="icon" @click="removeRecipeGroup(g.recipeId)">
                      <Trash2 class="h-4 w-4" />
                    </Button>
                  </td>
                </tr>
                <tr
                  v-for="e in g.entries"
                  :key="`re-${e.id}`"
                  class="border-b border-border last:border-0"
                >
                  <td class="px-3 py-2 pl-6">
                    <div class="text-muted-foreground">↳ {{ e.food_name }}</div>
                    <div class="text-xs text-muted-foreground">{{ e.food_unit }}</div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">
                    {{ formatNumber(e.quantity, e.quantity % 1 ? 1 : 0) }}
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">
                    {{ Math.round(e.calories) }}
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">
                    {{ formatNumber(e.protein, 1) }}
                  </td>
                  <td class="px-1" />
                </tr>
              </template>
            </template>
          </tbody>
        </table>
        <div class="px-3 py-3 border-t border-border flex justify-between font-semibold">
          <span>Total</span>
          <span>
            <span class="text-[hsl(var(--chart-blue))]">{{ formatNumber(totalCalories) }} kcal</span>
            <span class="ml-3 text-[hsl(var(--chart-violet))]">{{ formatNumber(totalProtein, 1) }} g</span>
          </span>
        </div>
      </Card>

      <Button variant="outline" @click="showDrawer = true">
        <Plus class="h-4 w-4" />
        Add food
      </Button>

      <Card>
        <div class="p-4 flex flex-col gap-3">
          <div class="flex items-center gap-3">
            <label class="w-32 text-sm text-muted-foreground">Weight</label>
            <Input
              v-model="weight"
              type="number"
              inputmode="decimal"
              min="0"
              step="0.1"
              placeholder="0"
              @blur="saveMetrics"
            />
            <span class="text-xs text-muted-foreground w-10">kg</span>
          </div>
          <div class="flex items-center gap-3">
            <label class="w-32 text-sm text-muted-foreground">Steps</label>
            <Input
              v-model="steps"
              type="number"
              inputmode="numeric"
              min="0"
              step="1"
              placeholder="0"
              @blur="saveMetrics"
            />
            <span class="text-xs text-muted-foreground w-10">steps</span>
          </div>
          <div class="flex items-center gap-3">
            <label class="w-32 text-sm text-muted-foreground">Target calories</label>
            <Input
              v-model="targetCalories"
              type="number"
              inputmode="numeric"
              min="1"
              step="50"
              placeholder="2000"
              @blur="saveTargetCalories"
            />
            <span class="text-xs text-muted-foreground w-10">kcal</span>
          </div>
          <div class="flex items-center gap-3">
            <label class="w-32 text-sm text-muted-foreground">Target protein</label>
            <Input
              v-model="targetProtein"
              type="number"
              inputmode="numeric"
              min="0"
              step="5"
              placeholder="0"
              @blur="saveTargetProtein"
            />
            <span class="text-xs text-muted-foreground w-10">g</span>
          </div>
        </div>
      </Card>
    </section>

    <section class="flex flex-col gap-3">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Progress
        </h2>
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
        <div v-for="i in 3" :key="i" class="h-48 rounded-lg bg-muted animate-pulse" />
      </div>
      <ProgressCharts
        v-else
        :data="metricsRange"
        :target="user?.target_calories ?? 0"
        :protein-target="user?.target_protein ?? 0"
      />
    </section>
  </div>

  <AddFoodDrawer
    v-if="showDrawer"
    :user-id="userId"
    :date="date"
    @close="showDrawer = false"
    @added="onFoodAdded"
  />
</template>
