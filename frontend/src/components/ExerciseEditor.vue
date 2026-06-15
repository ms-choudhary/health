<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '@/lib/api'
import type { ExercisePayload, ExerciseTag } from '@/lib/types'
import Dialog from '@/components/ui/Dialog.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'

const props = defineProps<{ open: boolean; exerciseId: number | null }>()
const emit = defineEmits<{
  'update:open': [v: boolean]
  saved: []
}>()

const name = ref<string>('')
const notes = ref<string>('')
const availableTags = ref<ExerciseTag[]>([])
const selectedTagIds = ref<number[]>([])
const saving = ref<boolean>(false)
const loading = ref<boolean>(false)
const errMsg = ref<string>('')

const isEdit = computed<boolean>(() => props.exerciseId != null)

async function loadTags(): Promise<void> {
  availableTags.value = await api.listExerciseTags()
}

function toggleTag(id: number): void {
  const i = selectedTagIds.value.indexOf(id)
  if (i === -1) selectedTagIds.value.push(id)
  else selectedTagIds.value.splice(i, 1)
}

function reset(): void {
  name.value = ''
  notes.value = ''
  selectedTagIds.value = []
  errMsg.value = ''
}

async function loadForEdit(id: number): Promise<void> {
  loading.value = true
  try {
    const exercise = await api.getExercise(id)
    name.value = exercise.name
    notes.value = exercise.notes
    selectedTagIds.value = exercise.tags.map((t) => t.id)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load exercise'
  } finally {
    loading.value = false
  }
}

function close(): void {
  emit('update:open', false)
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    reset()
    await loadTags()
    if (props.exerciseId != null) {
      await loadForEdit(props.exerciseId)
    }
  },
)

async function save(): Promise<void> {
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errMsg.value = 'Name required'
    return
  }
  const payload: ExercisePayload = {
    name: trimmedName,
    notes: notes.value.trim(),
    exercise_tag_ids: selectedTagIds.value,
  }
  saving.value = true
  errMsg.value = ''
  try {
    if (props.exerciseId != null) {
      await api.updateExercise(props.exerciseId, payload)
    } else {
      await api.createExercise(payload)
    }
    emit('saved')
    close()
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to save exercise'
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (props.open) {
    void loadTags()
    if (props.exerciseId != null) {
      void loadForEdit(props.exerciseId)
    }
  }
})
</script>

<template>
  <Dialog :open="open" :title="isEdit ? 'Edit exercise' : 'New exercise'" @update:open="(v) => emit('update:open', v)">
    <div v-if="loading" class="py-6 text-center text-sm text-muted-foreground">Loading…</div>
    <div v-else class="flex flex-col gap-3">
      <div>
        <label class="text-xs text-muted-foreground">Exercise name</label>
        <Input v-model="name" placeholder="e.g. Hack Squat" />
      </div>

      <div>
        <label class="text-xs text-muted-foreground">Notes</label>
        <Input v-model="notes" placeholder="e.g. Aim for 40" />
      </div>

      <div class="flex flex-col gap-2">
        <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
          Tags
        </div>
        <div v-if="availableTags.length === 0" class="text-xs text-muted-foreground">
          No tags yet — create them in the “Tags” tab first.
        </div>
        <div v-else class="flex flex-wrap gap-2">
          <button
            v-for="t in availableTags"
            :key="t.id"
            type="button"
            class="text-xs rounded-full border px-2.5 py-1 transition-colors"
            :class="selectedTagIds.includes(t.id)
              ? 'bg-primary text-primary-foreground border-primary'
              : 'border-border hover:bg-muted'"
            @click="toggleTag(t.id)"
          >
            {{ t.name }}
          </button>
        </div>
      </div>

      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="close">Cancel</Button>
        <Button :disabled="saving" @click="save">
          {{ saving ? 'Saving…' : isEdit ? 'Save' : 'Create' }}
        </Button>
      </div>
    </div>
  </Dialog>
</template>
