<script setup lang="ts">
import { cn } from '@/lib/utils'

defineProps<{ class?: string }>()
</script>

<template>
  <div
    :class="cn(
      'rounded-2xl border border-border bg-card text-card-foreground shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)]',
      $props.class,
    )"
  >
    <slot />
  </div>
</template>
