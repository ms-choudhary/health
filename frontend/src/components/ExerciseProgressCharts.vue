<script setup lang="ts">
import { computed } from 'vue'
import { Bar } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Tooltip,
  type ChartData,
  type ChartOptions,
  type TooltipItem,
} from 'chart.js'
import type { ExerciseProgress } from '@/lib/types'
import Card from './ui/Card.vue'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip)

const props = defineProps<{ data: ExerciseProgress[] }>()

function getCssVar(name: string): string {
  if (typeof window === 'undefined') return '#7c3aed'
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim()
  return v ? `hsl(${v})` : '#7c3aed'
}

interface ExerciseChart {
  name: string
  chartData: ChartData<'bar'>
  options: ChartOptions<'bar'>
}

const charts = computed<ExerciseChart[]>(() => {
  const violet = getCssVar('--chart-violet')
  return props.data.map((ex) => {
    const points = ex.points
    const chartData: ChartData<'bar'> = {
      labels: ex.points.map((p) => p.date.slice(5)),
      datasets: [
        {
          label: 'Volume',
          data: ex.points.map((p) => p.total_volume),
          backgroundColor: violet,
          borderRadius: 4,
        },
      ],
    }
    const options: ChartOptions<'bar'> = {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            label: (item: TooltipItem<'bar'>) => `Volume: ${Math.round(item.parsed.y ?? 0)}`,
            footer: (items: TooltipItem<'bar'>[]) => {
              const first = items[0]
              if (first === undefined) return ''
              return points[first.dataIndex]?.breakdown ?? ''
            },
          },
        },
      },
      scales: {
        x: { ticks: { font: { size: 10 } } },
        y: { beginAtZero: true, ticks: { font: { size: 10 } } },
      },
    }
    return { name: ex.exercise_name, chartData, options }
  })
})
</script>

<template>
  <div v-if="data.length === 0" class="text-center py-6 text-sm text-muted-foreground">
    No exercise data in this period.
  </div>
  <div v-else class="flex flex-col gap-4">
    <Card v-for="c in charts" :key="c.name">
      <div class="p-3">
        <div class="text-sm font-medium mb-2">{{ c.name }}</div>
        <div class="h-40"><Bar :data="c.chartData" :options="c.options" /></div>
      </div>
    </Card>
  </div>
</template>
