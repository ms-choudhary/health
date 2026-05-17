<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ consumed: number; target: number; size?: number }>(),
  { size: 56 },
)

const sw = computed(() => props.size * 0.12)
const r = computed(() => (props.size - sw.value) / 2)
const cx = computed(() => props.size / 2)
const cy = computed(() => props.size / 2)
const circumference = computed(() => 2 * Math.PI * r.value)

const pct = computed(() => {
  if (props.target <= 0) return 0
  return Math.min(100, Math.round((props.consumed / props.target) * 100))
})

const filled = computed(() => (pct.value / 100) * circumference.value)

const color = computed(() => {
  if (pct.value >= 100) return 'hsl(var(--destructive))'
  if (pct.value >= 80) return 'hsl(var(--chart-amber))'
  return 'hsl(var(--chart-blue))'
})

const label = computed(() => (props.target > 0 ? `${pct.value}%` : '—'))
</script>

<template>
  <svg
    :width="size"
    :height="size"
    :viewBox="`0 0 ${size} ${size}`"
    role="img"
    :aria-label="`${consumed} of ${target} calories consumed`"
  >
    <circle
      :cx="cx"
      :cy="cy"
      :r="r"
      fill="none"
      stroke="hsl(var(--muted))"
      :stroke-width="sw"
    />
    <circle
      :cx="cx"
      :cy="cy"
      :r="r"
      fill="none"
      :stroke="color"
      :stroke-width="sw"
      stroke-linecap="round"
      :stroke-dasharray="`${filled} ${circumference}`"
      :transform="`rotate(-90 ${cx} ${cy})`"
    />
    <text
      :x="cx"
      :y="cy + 4"
      text-anchor="middle"
      :font-size="size * 0.22"
      font-weight="700"
      fill="hsl(var(--foreground))"
    >
      {{ label }}
    </text>
  </svg>
</template>
