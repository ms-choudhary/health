<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import type { ExerciseWithTags, ExerciseTagWithCount } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Badge from '@/components/ui/Badge.vue'
import ExerciseEditor from '@/components/ExerciseEditor.vue'
import ExerciseTagEditor from '@/components/ExerciseTagEditor.vue'
import { ChevronLeft, Search, Trash2, Plus, Pencil } from 'lucide-vue-next'

type Tab = 'exercises' | 'tags'

const router = useRouter()
const tab = ref<Tab>('exercises')
const query = ref<string>('')

const exercises = ref<ExerciseWithTags[]>([])
const loadingExercises = ref<boolean>(true)

const exerciseTags = ref<ExerciseTagWithCount[]>([])
const loadingTags = ref<boolean>(true)

const selectedTag = ref<ExerciseTagWithCount | null>(null)
const taggedExercises = ref<ExerciseWithTags[]>([])
const loadingTaggedExercises = ref<boolean>(false)

const showExerciseEditor = ref<boolean>(false)
const editingExerciseId = ref<number | null>(null)

const showTagEditor = ref<boolean>(false)

let searchTimer: number | undefined

async function loadExercises(): Promise<void> {
  loadingExercises.value = true
  try {
    exercises.value = await api.listExercises(query.value)
  } finally {
    loadingExercises.value = false
  }
}

async function loadTags(): Promise<void> {
  loadingTags.value = true
  try {
    exerciseTags.value = await api.listExerciseTags()
  } finally {
    loadingTags.value = false
  }
}

async function loadCurrent(): Promise<void> {
  if (tab.value === 'exercises') await loadExercises()
  else await loadTags()
}

watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadCurrent, 200)
})

watch(tab, () => {
  query.value = ''
  selectedTag.value = null
  void loadCurrent()
})

function openNewExercise(): void {
  editingExerciseId.value = null
  showExerciseEditor.value = true
}

function openEditExercise(id: number): void {
  editingExerciseId.value = id
  showExerciseEditor.value = true
}

function onExerciseSaved(): void {
  void loadExercises()
}

async function deleteExercise(id: number): Promise<void> {
  if (!confirm('Delete this exercise from the library? Past sets are unaffected.')) return
  await api.deleteExercise(id)
  await loadExercises()
}

function openNewTag(): void {
  showTagEditor.value = true
}

function onTagSaved(): void {
  void loadTags()
}

async function deleteTag(id: number): Promise<void> {
  if (!confirm('Delete this tag? It will be removed from all exercises.')) return
  await api.deleteExerciseTag(id)
  if (selectedTag.value?.id === id) selectedTag.value = null
  await loadTags()
}

async function openTag(t: ExerciseTagWithCount): Promise<void> {
  selectedTag.value = t
  loadingTaggedExercises.value = true
  try {
    taggedExercises.value = await api.exercisesByTag(t.id)
  } finally {
    loadingTaggedExercises.value = false
  }
}

onMounted(() => {
  void loadCurrent()
})
</script>

<template>
  <div class="max-w-lg mx-auto p-4 sm:p-6 flex flex-col gap-4">
    <header class="flex items-center gap-2">
      <Button variant="ghost" size="icon" @click="router.push('/')">
        <ChevronLeft class="h-5 w-5" />
      </Button>
      <h1 class="text-xl font-bold">Exercise Library</h1>
    </header>

    <div class="flex gap-2">
      <Button :variant="tab === 'exercises' ? 'default' : 'outline'" size="sm" @click="tab = 'exercises'">
        Exercises
      </Button>
      <Button :variant="tab === 'tags' ? 'default' : 'outline'" size="sm" @click="tab = 'tags'">
        Tags
      </Button>
    </div>

    <div v-if="tab === 'exercises'" class="relative">
      <Input v-model="query" type="search" placeholder="Search exercises…" />
      <Search class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
    </div>

    <template v-if="tab === 'exercises'">
      <div v-if="loadingExercises" class="flex flex-col gap-2">
        <div v-for="i in 4" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
      </div>

      <div v-else-if="exercises.length === 0" class="text-center py-10 text-muted-foreground text-sm">
        <template v-if="query">No exercise matches "{{ query }}".</template>
        <template v-else>No exercises yet — tap "Add exercise" below.</template>
      </div>

      <div v-else class="flex flex-col gap-2">
        <Card v-for="e in exercises" :key="e.id">
          <div class="p-3 flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="font-medium truncate">{{ e.name }}</div>
              <div v-if="e.notes" class="text-xs text-muted-foreground italic truncate">{{ e.notes }}</div>
              <div v-if="e.tags.length" class="mt-1 flex flex-wrap gap-1">
                <Badge v-for="t in e.tags" :key="t.id" variant="outline">{{ t.name }}</Badge>
              </div>
            </div>
            <Button variant="ghost" size="icon" @click="openEditExercise(e.id)">
              <Pencil class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" @click="deleteExercise(e.id)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </Card>
      </div>

      <Button class="mt-2" @click="openNewExercise">
        <Plus class="h-4 w-4" />
        Add exercise
      </Button>
    </template>

    <template v-else>
      <template v-if="selectedTag">
        <Button variant="ghost" size="sm" class="self-start" @click="selectedTag = null">
          ← All tags
        </Button>
        <div class="text-sm font-medium">Exercises tagged “{{ selectedTag.name }}”</div>
        <div v-if="loadingTaggedExercises" class="flex flex-col gap-2">
          <div v-for="i in 3" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
        </div>
        <div v-else-if="taggedExercises.length === 0" class="text-center py-10 text-muted-foreground text-sm">
          No exercises have this tag yet.
        </div>
        <div v-else class="flex flex-col gap-2">
          <Card v-for="e in taggedExercises" :key="e.id">
            <div class="p-3">
              <div class="font-medium truncate">{{ e.name }}</div>
              <div v-if="e.notes" class="text-xs text-muted-foreground italic truncate">{{ e.notes }}</div>
            </div>
          </Card>
        </div>
      </template>

      <template v-else>
        <div v-if="loadingTags" class="flex flex-col gap-2">
          <div v-for="i in 4" :key="i" class="h-12 rounded-lg bg-muted animate-pulse" />
        </div>
        <div v-else-if="exerciseTags.length === 0" class="text-center py-10 text-muted-foreground text-sm">
          No tags yet — tap "Add tag" below.
        </div>
        <div v-else class="flex flex-col gap-2">
          <Card v-for="t in exerciseTags" :key="t.id">
            <div class="p-3 flex items-center gap-3">
              <button class="flex-1 min-w-0 text-left" @click="openTag(t)">
                <div class="font-medium truncate">{{ t.name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ t.exercise_count }} exercise{{ t.exercise_count === 1 ? '' : 's' }}
                </div>
              </button>
              <Button variant="ghost" size="icon" @click="deleteTag(t.id)">
                <Trash2 class="h-4 w-4" />
              </Button>
            </div>
          </Card>
        </div>

        <Button class="mt-2" @click="openNewTag">
          <Plus class="h-4 w-4" />
          Add tag
        </Button>
      </template>
    </template>
  </div>

  <ExerciseEditor
    v-model:open="showExerciseEditor"
    :exercise-id="editingExerciseId"
    @saved="onExerciseSaved"
  />

  <ExerciseTagEditor
    v-model:open="showTagEditor"
    @saved="onTagSaved"
  />
</template>
