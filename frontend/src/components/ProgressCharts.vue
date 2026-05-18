<script setup lang="ts">
import { computed } from 'vue'
import { Line, Bar } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Tooltip,
  Legend,
  Filler,
  type ChartData,
  type ChartOptions,
} from 'chart.js'
import type { DailyMetric } from '@/lib/types'
import Card from './ui/Card.vue'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Tooltip,
  Legend,
  Filler,
)

const props = defineProps<{ data: DailyMetric[]; target: number }>()

const labels = computed(() => props.data.map((d) => d.date.slice(5)))

function getCssVar(name: string): string {
  if (typeof window === 'undefined') return '#3b82f6'
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v ? `hsl(${v})` : '#3b82f6'
}

const blue = computed(() => getCssVar('--chart-blue'))
const amber = computed(() => getCssVar('--chart-amber'))
const green = computed(() => getCssVar('--chart-green'))
const violet = computed(() => getCssVar('--chart-violet'))

const caloriesData = computed<ChartData<'line'>>(() => ({
  labels: labels.value,
  datasets: [
    {
      label: 'Consumed',
      data: props.data.map((d) => Math.round(d.calories_consumed)),
      borderColor: blue.value,
      backgroundColor: blue.value,
      tension: 0.35,
      pointRadius: 3,
      fill: false,
    },
    {
      label: 'Target',
      data: props.data.map(() => (props.target > 0 ? props.target : null)),
      borderColor: amber.value,
      backgroundColor: amber.value,
      borderDash: [6, 4],
      tension: 0,
      pointRadius: 0,
      fill: false,
    },
  ],
}))

const weightData = computed<ChartData<'line'>>(() => ({
  labels: labels.value,
  datasets: [
    {
      label: 'Weight',
      data: props.data.map((d) => d.weight ?? null),
      borderColor: green.value,
      backgroundColor: green.value,
      tension: 0.35,
      pointRadius: 3,
      fill: false,
    },
  ],
}))

const stepsData = computed<ChartData<'bar'>>(() => ({
  labels: labels.value,
  datasets: [
    {
      label: 'Steps',
      data: props.data.map((d) => d.steps ?? 0),
      backgroundColor: violet.value,
      borderRadius: 4,
    },
  ],
}))

const lineOpts: ChartOptions<'line'> = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: true, position: 'bottom', labels: { boxWidth: 10, font: { size: 11 } } },
    tooltip: { mode: 'index', intersect: false },
  },
  scales: {
    x: { ticks: { font: { size: 10 } } },
    y: { beginAtZero: false, ticks: { font: { size: 10 } } },
  },
  interaction: { mode: 'nearest', intersect: false },
}

const lineOptsZero: ChartOptions<'line'> = {
  ...lineOpts,
  plugins: { ...lineOpts.plugins, legend: { display: false } },
  scales: {
    x: { ticks: { font: { size: 10 } } },
    y: { beginAtZero: false, ticks: { font: { size: 10 } } },
  },
}

const barOpts: ChartOptions<'bar'> = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { display: false } },
  scales: {
    x: { ticks: { font: { size: 10 } } },
    y: { beginAtZero: true, ticks: { font: { size: 10 } } },
  },
}

const hasData = computed(() => props.data.length > 0)
</script>

<template>
  <div class="flex flex-col gap-3">
    <Card>
      <div class="p-4 pb-2">
        <div class="text-sm font-semibold">Calories</div>
      </div>
      <div class="px-3 pb-3">
        <div class="h-40">
          <Line v-if="hasData" :data="caloriesData" :options="lineOpts" />
          <div v-else class="h-full grid place-items-center text-muted-foreground text-sm">
            Not enough data
          </div>
        </div>
      </div>
    </Card>

    <Card>
      <div class="p-4 pb-2">
        <div class="text-sm font-semibold">Weight (kg)</div>
      </div>
      <div class="px-3 pb-3">
        <div class="h-40">
          <Line v-if="hasData" :data="weightData" :options="lineOptsZero" />
          <div v-else class="h-full grid place-items-center text-muted-foreground text-sm">
            Not enough data
          </div>
        </div>
      </div>
    </Card>

    <Card>
      <div class="p-4 pb-2">
        <div class="text-sm font-semibold">Steps</div>
      </div>
      <div class="px-3 pb-3">
        <div class="h-40">
          <Bar v-if="hasData" :data="stepsData" :options="barOpts" />
          <div v-else class="h-full grid place-items-center text-muted-foreground text-sm">
            Not enough data
          </div>
        </div>
      </div>
    </Card>
  </div>
</template>
