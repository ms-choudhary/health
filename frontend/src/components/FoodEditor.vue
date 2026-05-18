<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { api } from '@/lib/api'
import type { Food } from '@/lib/types'
import Dialog from '@/components/ui/Dialog.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'

const UNITS = ['g', 'ml', 'oz', 'piece', 'tbsp', 'cup', 'serving']

const props = defineProps<{ open: boolean; food: Food | null }>()
const emit = defineEmits<{
  'update:open': [v: boolean]
  saved: []
}>()

const isEdit = computed<boolean>(() => props.food != null)

const name = ref<string>('')
const unit = ref<string>('g')
const calories = ref<string>('')
const saving = ref<boolean>(false)
const errMsg = ref<string>('')
const aiHint = ref<string>('')
const aiLoading = ref<boolean>(false)
const aiError = ref<string>('')

function reset(): void {
  name.value = props.food?.name ?? ''
  unit.value = props.food?.unit ?? 'g'
  calories.value = props.food != null ? String(props.food.calories_per_unit) : ''
  errMsg.value = ''
  aiHint.value = ''
  aiError.value = ''
  aiLoading.value = false
}

watch(
  () => props.open,
  (open) => {
    if (open) reset()
  },
)

async function fetchCalorieHint(): Promise<void> {
  const trimmed = name.value.trim()
  if (!trimmed) return
  aiHint.value = ''
  aiError.value = ''
  aiLoading.value = true
  try {
    const res = await api.calorieHint(trimmed)
    aiHint.value = res.hint
  } catch {
    aiError.value = 'Could not fetch AI hint.'
  } finally {
    aiLoading.value = false
  }
}

async function save(): Promise<void> {
  const cal = Number(calories.value)
  if (!Number.isFinite(cal) || cal < 0) {
    errMsg.value = 'Enter a non-negative calorie value'
    return
  }
  if (!isEdit.value && !name.value.trim()) {
    errMsg.value = 'Name required'
    return
  }
  saving.value = true
  errMsg.value = ''
  try {
    if (props.food != null) {
      await api.updateFood(props.food.id, { calories_per_unit: cal })
    } else {
      await api.createFood({
        name: name.value.trim(),
        unit: unit.value,
        calories_per_unit: cal,
      })
    }
    emit('saved')
    emit('update:open', false)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to save food'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    :open="open"
    :title="isEdit ? 'Edit food' : 'New food'"
    @update:open="(v) => emit('update:open', v)"
  >
    <div class="flex flex-col gap-3">
      <template v-if="isEdit">
        <div>
          <div class="text-xs text-muted-foreground">Food</div>
          <div class="font-medium">{{ name }}</div>
        </div>
      </template>
      <template v-else>
        <div class="flex gap-2">
          <Input v-model="name" placeholder="Food name" class="flex-1" />
          <Button
            type="button"
            variant="outline"
            size="sm"
            :disabled="aiLoading || !name.trim()"
            @click="fetchCalorieHint"
            title="Ask AI for calorie info"
          >
            <span v-if="aiLoading" class="animate-pulse">…</span>
            <span v-else>✨ AI</span>
          </Button>
        </div>
        <p v-if="aiHint" class="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground leading-snug">
          {{ aiHint }}
        </p>
        <p v-if="aiError" class="text-xs text-destructive">{{ aiError }}</p>
      </template>

      <div class="flex gap-2">
        <Input
          v-model="calories"
          type="number"
          inputmode="decimal"
          placeholder="Calories"
          min="0"
          step="0.1"
        />
        <template v-if="isEdit">
          <div class="h-9 rounded-md border border-input bg-muted px-3 text-sm grid place-items-center text-muted-foreground">
            {{ unit }}
          </div>
        </template>
        <template v-else>
          <select
            v-model="unit"
            class="h-9 rounded-md border border-input bg-background px-2 text-sm"
          >
            <option v-for="u in UNITS" :key="u" :value="u">{{ u }}</option>
          </select>
        </template>
      </div>
      <p class="text-xs text-muted-foreground">Calories per 1 {{ unit }}</p>
      <p v-if="isEdit" class="text-xs text-muted-foreground">
        Changing this updates all past log entries for {{ name }}.
      </p>
      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="emit('update:open', false)">Cancel</Button>
        <Button :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Add' }}
        </Button>
      </div>
    </div>
  </Dialog>
</template>
