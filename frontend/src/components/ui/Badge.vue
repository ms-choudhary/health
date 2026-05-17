<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

type Variant = 'default' | 'secondary' | 'outline' | 'destructive'

const props = withDefaults(defineProps<{ variant?: Variant }>(), {
  variant: 'default',
})

const cls = computed(() => {
  const variants: Record<Variant, string> = {
    default: 'bg-primary text-primary-foreground',
    secondary: 'bg-secondary text-secondary-foreground',
    outline: 'border border-border text-foreground',
    destructive: 'bg-destructive text-destructive-foreground',
  }
  return cn(
    'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
    variants[props.variant],
  )
})
</script>

<template>
  <span :class="cls"><slot /></span>
</template>
