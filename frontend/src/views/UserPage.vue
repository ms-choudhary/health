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
import type { LogEntry, DailyMetric, AddLogPayload } from '@/lib/types'
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
let savingMetrics = false

const period = ref<Period>('w')
const metricsRange = ref<DailyMetric[]>([])
const loadingCharts = ref(false)

const showDrawer = ref(false)

const isToday = computed(() => dateFnsIsToday(parseISO(date.value)))
const displayDate = computed(() => format(parseISO(date.value), 'd MMM yyyy'))
const totalCalories = computed(() =>
  Math.round(entries.value.reduce((s, e) => s + e.calories, 0)),
)
const user = computed(() => userStore.findById(props.userId))

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
    targetCalories.value = m?.target_calories != null ? String(m.target_calories) : ''
  } finally {
    loadingLog.value = false
  }
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
    const t = parseOptionalNumber(targetCalories.value)
    await api.saveMetrics(props.userId, {
      date: date.value,
      weight: w,
      steps: s != null ? Math.round(s) : null,
      target_calories: t != null ? Math.round(t) : null,
    })
    await loadCharts()
  } finally {
    savingMetrics = false
  }
}

async function removeEntry(id: number) {
  await api.deleteLog(props.userId, id)
  entries.value = entries.value.filter((e) => e.id !== id)
}

async function onFoodAdded(_payload: AddLogPayload) {
  showDrawer.value = false
  await loadLog()
}

watch(date, loadLog)
watch(period, loadCharts)

onMounted(async () => {
  if (userStore.users.length === 0) await userStore.load()
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
              <th class="w-9" />
            </tr>
          </thead>
          <tbody>
            <tr v-for="e in entries" :key="e.id" class="border-b border-border last:border-0">
              <td class="px-3 py-2">
                <div>{{ e.food_name }}</div>
                <div class="text-xs text-muted-foreground">{{ e.food_unit }}</div>
              </td>
              <td class="text-right px-3 py-2">{{ formatNumber(e.quantity, e.quantity % 1 ? 1 : 0) }}</td>
              <td class="text-right px-3 py-2">{{ Math.round(e.calories) }}</td>
              <td class="px-1">
                <Button variant="ghost" size="icon" @click="removeEntry(e.id)">
                  <Trash2 class="h-4 w-4" />
                </Button>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="px-3 py-3 border-t border-border flex justify-between font-semibold">
          <span>Total</span>
          <span class="text-[hsl(var(--chart-blue))]">{{ formatNumber(totalCalories) }} kcal</span>
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
              min="0"
              step="50"
              placeholder="2000"
              @blur="saveMetrics"
            />
            <span class="text-xs text-muted-foreground w-10">kcal</span>
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
      <ProgressCharts v-else :data="metricsRange" />
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
