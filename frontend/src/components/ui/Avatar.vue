<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ initials: string; size?: number; seed?: string | number }>(),
  { size: 40, seed: '' },
)

const bgIndex = computed(() => {
  const key = String(props.seed || props.initials)
  let hash = 0
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) >>> 0
  return hash % 6
})

const palette = [
  { bg: '#dbeafe', fg: '#1d4ed8' },
  { bg: '#fce7f3', fg: '#be185d' },
  { bg: '#d1fae5', fg: '#065f46' },
  { bg: '#fef3c7', fg: '#92400e' },
  { bg: '#e0e7ff', fg: '#4338ca' },
  { bg: '#ffedd5', fg: '#c2410c' },
]

const style = computed(() => {
  const c = palette[bgIndex.value]
  return {
    background: c.bg,
    color: c.fg,
    width: `${props.size}px`,
    height: `${props.size}px`,
    fontSize: `${Math.round(props.size * 0.36)}px`,
  }
})
</script>

<template>
  <div
    class="inline-flex items-center justify-center rounded-full font-bold select-none flex-shrink-0"
    :style="style"
  >
    {{ initials.slice(0, 2).toUpperCase() }}
  </div>
</template>
