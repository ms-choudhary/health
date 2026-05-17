<script setup lang="ts">
import { cn } from '@/lib/utils'

withDefaults(
  defineProps<{
    modelValue?: string | number | null
    type?: string
    placeholder?: string
    disabled?: boolean
    min?: string | number
    step?: string | number
    inputmode?: 'text' | 'search' | 'email' | 'tel' | 'url' | 'none' | 'numeric' | 'decimal'
  }>(),
  { type: 'text', disabled: false },
)

defineEmits<{
  'update:modelValue': [value: string]
}>()
</script>

<template>
  <input
    :type="type"
    :value="modelValue ?? ''"
    :placeholder="placeholder"
    :disabled="disabled"
    :min="min"
    :step="step"
    :inputmode="inputmode"
    :class="cn(
      'flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm',
      'placeholder:text-muted-foreground',
      'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
      'disabled:cursor-not-allowed disabled:opacity-50',
    )"
    @input="$emit('update:modelValue', ($event.target as HTMLInputElement).value)"
  />
</template>
