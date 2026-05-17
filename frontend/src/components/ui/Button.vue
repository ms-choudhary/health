<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'

type Variant = 'default' | 'outline' | 'ghost' | 'destructive' | 'secondary'
type Size = 'sm' | 'md' | 'icon'

const props = withDefaults(
  defineProps<{
    variant?: Variant
    size?: Size
    disabled?: boolean
    type?: 'button' | 'submit' | 'reset'
  }>(),
  { variant: 'default', size: 'md', disabled: false, type: 'button' },
)

const cls = computed(() => {
  const variants: Record<Variant, string> = {
    default: 'bg-primary text-primary-foreground hover:opacity-90',
    outline: 'border border-border bg-transparent hover:bg-muted',
    ghost: 'bg-transparent hover:bg-muted text-foreground',
    destructive: 'bg-destructive text-destructive-foreground hover:opacity-90',
    secondary: 'bg-secondary text-secondary-foreground hover:opacity-90',
  }
  const sizes: Record<Size, string> = {
    sm: 'h-8 px-3 text-xs rounded-md',
    md: 'h-9 px-4 text-sm rounded-md',
    icon: 'h-9 w-9 rounded-md',
  }
  return cn(
    'inline-flex items-center justify-center gap-1.5 font-medium transition-colors',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
    'disabled:opacity-40 disabled:pointer-events-none',
    variants[props.variant],
    sizes[props.size],
  )
})
</script>

<template>
  <button :type="type" :class="cls" :disabled="disabled">
    <slot />
  </button>
</template>
