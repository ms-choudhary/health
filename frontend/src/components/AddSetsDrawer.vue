<script setup lang="ts">
import { reactive, ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/lib/api'
import type { ExerciseWithTags } from '@/lib/types'
import Button from './ui/Button.vue'
import { Plus, Minus, Trash2, X } from 'lucide-vue-next'

interface Draft {
  id: number
  weight: number
  reps: number
}

const props = defineProps<{ userId: number; date: string; exercise: ExerciseWithTags }>()
const emit = defineEmits<{
  close: []
  added: []
}>()

let counter = 0
const sets = reactive<Draft[]>([])
const saving = ref<boolean>(false)
const loading = ref<boolean>(true)
const errMsg = ref<string>('')

function addSet(): void {
  const last = sets[sets.length - 1]
  sets.push({ id: counter++, weight: last?.weight ?? 20, reps: last?.reps ?? 8 })
}

function removeSet(id: number): void {
  const i = sets.findIndex((s) => s.id === id)
  if (i >= 0) sets.splice(i, 1)
  if (sets.length === 0) addSet()
}

function step(s: Draft, field: 'weight' | 'reps', delta: number): void {
  if (field === 'weight') s.weight = Math.max(0, Number((s.weight + delta).toFixed(1)))
  else s.reps = Math.max(0, s.reps + delta)
}

async function create(): Promise<void> {
  saving.value = true
  errMsg.value = ''
  try {
    await api.addSets(props.userId, {
      exercise_id: props.exercise.id,
      date: props.date,
      sets: sets.map((s) => ({ weight: s.weight, reps: s.reps })),
    })
    emit('added')
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to add sets'
  } finally {
    saving.value = false
  }
}

function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close')
}

const vvHeight = ref<number>(typeof window !== 'undefined' ? window.innerHeight : 0)
const vvTop = ref<number>(0)

function onViewportChange(): void {
  const vv = window.visualViewport
  if (!vv) return
  vvHeight.value = vv.height
  vvTop.value = vv.offsetTop
}

onMounted(async () => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', onViewportChange)
    vv.addEventListener('scroll', onViewportChange)
    onViewportChange()
  }
  try {
    const last = await api.lastSets(props.userId, props.exercise.id)
    for (const s of last) sets.push({ id: counter++, weight: s.weight, reps: s.reps })
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : ''
  } finally {
    if (sets.length === 0) sets.push({ id: counter++, weight: 20, reps: 8 })
    loading.value = false
  }
})

onUnmounted(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.removeEventListener('resize', onViewportChange)
    vv.removeEventListener('scroll', onViewportChange)
  }
})
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed left-0 right-0 z-50 flex items-end justify-center"
      :style="{ top: `${vvTop}px`, height: `${vvHeight}px` }"
      @click.self="emit('close')"
    >
      <div class="absolute inset-0 bg-black/50" />
      <div
        class="relative w-full max-w-lg bg-card text-card-foreground border-t border-border rounded-t-2xl p-4 pb-8 flex flex-col gap-3 max-h-[calc(100%-2rem)] overflow-y-auto"
      >
        <div class="mx-auto h-1 w-10 rounded-full bg-border" />

        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <div class="text-xs text-muted-foreground">Add sets</div>
            <h2 class="font-semibold text-lg truncate">{{ exercise.name }}</h2>
          </div>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div v-if="loading" class="py-6 text-center text-sm text-muted-foreground">Loading…</div>

        <template v-else>
          <div class="flex flex-col gap-2">
            <div v-for="(s, idx) in sets" :key="s.id" class="rounded-lg border border-border p-3">
              <div class="flex items-center justify-between mb-2">
                <span class="text-xs font-medium text-muted-foreground uppercase tracking-wide">Set {{ idx + 1 }}</span>
                <Button v-if="sets.length > 1" variant="ghost" size="icon" @click="removeSet(s.id)">
                  <Trash2 class="h-4 w-4" />
                </Button>
              </div>
              <div class="grid grid-cols-2 gap-3">
                <div>
                  <div class="text-xs text-muted-foreground mb-1">Weight (kg)</div>
                  <div class="flex items-stretch gap-1">
                    <Button variant="outline" size="icon" @click="step(s, 'weight', -2.5)">
                      <Minus class="h-4 w-4" />
                    </Button>
                    <div class="flex-1 grid place-items-center rounded-md border border-input bg-card font-mono font-semibold">
                      {{ s.weight }}
                    </div>
                    <Button variant="outline" size="icon" @click="step(s, 'weight', 2.5)">
                      <Plus class="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <div>
                  <div class="text-xs text-muted-foreground mb-1">Reps</div>
                  <div class="flex items-stretch gap-1">
                    <Button variant="outline" size="icon" @click="step(s, 'reps', -1)">
                      <Minus class="h-4 w-4" />
                    </Button>
                    <div class="flex-1 grid place-items-center rounded-md border border-input bg-card font-mono font-semibold">
                      {{ s.reps }}
                    </div>
                    <Button variant="outline" size="icon" @click="step(s, 'reps', 1)">
                      <Plus class="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <Button variant="outline" class="w-full border-dashed" @click="addSet">
            <Plus class="h-4 w-4" /> Add another set
          </Button>

          <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

          <Button class="w-full" :disabled="saving" @click="create">
            {{ saving ? 'Saving…' : 'Create' }}
          </Button>
        </template>
      </div>
    </div>
  </Teleport>
</template>
