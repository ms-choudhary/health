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

async function saveTargetProtein(): Promise<void> {
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
