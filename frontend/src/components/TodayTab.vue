<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { format } from 'date-fns'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type {
  FoodTagWithCount,
  RecipeListItem,
  ExerciseTagWithCount,
  ExerciseWithTags,
} from '@/lib/types'
import Button from '@/components/ui/Button.vue'
import AddRecipeDrawer from '@/components/AddRecipeDrawer.vue'
import CustomRecipeDrawer from '@/components/CustomRecipeDrawer.vue'
import AddSetsDrawer from '@/components/AddSetsDrawer.vue'

const props = defineProps<{ userId: number }>()

const today = format(new Date(), 'yyyy-MM-dd')
const dayName = format(new Date(), 'EEEE')
const dateLabel = format(new Date(), 'MMMM d, yyyy')

const foodTags = ref<FoodTagWithCount[]>([])
const exerciseTags = ref<ExerciseTagWithCount[]>([])
const loadingTags = ref<boolean>(true)

const selected = ref<{ kind: 'food' | 'exercise'; name: string } | null>(null)
const recipes = ref<RecipeListItem[]>([])
const exercises = ref<ExerciseWithTags[]>([])
const loadingItems = ref<boolean>(false)

const showRecipeDrawer = ref<boolean>(false)
const pickedRecipe = ref<RecipeListItem | null>(null)

const showCustomDrawer = ref<boolean>(false)
const customizeRecipeId = ref<number | null>(null)

const showSetsDrawer = ref<boolean>(false)
const pickedExercise = ref<ExerciseWithTags | null>(null)

async function loadTags(): Promise<void> {
  loadingTags.value = true
  try {
    const [food, exercise] = await Promise.all([api.listFoodTags(), api.listExerciseTags()])
    foodTags.value = food
    exerciseTags.value = exercise
  } finally {
    loadingTags.value = false
  }
}

async function openFoodTag(t: FoodTagWithCount): Promise<void> {
  selected.value = { kind: 'food', name: t.name }
  loadingItems.value = true
  try {
    recipes.value = await api.recipesByFoodTag(t.id)
  } finally {
    loadingItems.value = false
  }
}

async function openExerciseTag(t: ExerciseTagWithCount): Promise<void> {
  selected.value = { kind: 'exercise', name: t.name }
  loadingItems.value = true
  try {
    exercises.value = await api.exercisesByTag(t.id)
  } finally {
    loadingItems.value = false
  }
}

function addRecipeToLog(recipe: RecipeListItem): void {
  pickedRecipe.value = recipe
  showRecipeDrawer.value = true
}

function customizeRecipe(recipe: RecipeListItem): void {
  customizeRecipeId.value = recipe.id
  showCustomDrawer.value = true
}

function addSetsToLog(exercise: ExerciseWithTags): void {
  pickedExercise.value = exercise
  showSetsDrawer.value = true
}

function onRecipeAdded(): void {
  showRecipeDrawer.value = false
  pickedRecipe.value = null
}

function onCustomized(): void {
  showCustomDrawer.value = false
  customizeRecipeId.value = null
}

function onSetsAdded(): void {
  showSetsDrawer.value = false
  pickedExercise.value = null
}

onMounted(loadTags)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div>
      <div class="font-mono text-xs uppercase tracking-widest text-primary">{{ dayName }}</div>
      <div class="text-3xl font-bold tracking-tight">{{ dateLabel }}</div>
    </div>

    <template v-if="selected && selected.kind === 'food'">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold lowercase">{{ selected.name }}</h2>
        <Button variant="secondary" size="sm" @click="selected = null">← Back</Button>
      </div>
      <div v-if="loadingItems" class="flex flex-col gap-2">
        <div v-for="i in 3" :key="i" class="h-20 rounded-xl bg-card animate-pulse" />
      </div>
      <div v-else-if="recipes.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
        No recipes with this tag yet.
      </div>
      <div v-else class="flex flex-col gap-2.5">
        <div
          v-for="r in recipes"
          :key="r.id"
          class="rounded-2xl border border-border bg-card p-4 flex items-start justify-between gap-3 shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)]"
        >
          <div class="min-w-0">
            <div class="font-semibold truncate">{{ r.name }}</div>
            <div class="font-mono text-xs text-muted-foreground mt-1">
              {{ Math.round(r.total_calories) }} cal · {{ formatNumber(r.total_protein, 1) }} g / serving
            </div>
            <div v-if="r.food_tags.length" class="flex flex-wrap gap-1.5 mt-2">
              <span
                v-for="t in r.food_tags"
                :key="t.id"
                class="font-mono text-[10px] tracking-wide text-muted-foreground bg-secondary border border-border rounded px-2 py-1"
              >
                {{ t.name }}
              </span>
            </div>
          </div>
          <div class="flex flex-col gap-2 shrink-0">
            <Button size="sm" @click="addRecipeToLog(r)">Add to log</Button>
            <Button size="sm" variant="secondary" @click="customizeRecipe(r)">Customize</Button>
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="selected && selected.kind === 'exercise'">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold lowercase">{{ selected.name }}</h2>
        <Button variant="secondary" size="sm" @click="selected = null">← Back</Button>
      </div>
      <div v-if="loadingItems" class="flex flex-col gap-2">
        <div v-for="i in 3" :key="i" class="h-20 rounded-xl bg-card animate-pulse" />
      </div>
      <div v-else-if="exercises.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
        No exercises with this tag yet.
      </div>
      <div v-else class="flex flex-col gap-2.5">
        <div
          v-for="e in exercises"
          :key="e.id"
          class="rounded-2xl border border-border bg-card p-4 flex items-start justify-between gap-3 shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)]"
        >
          <div class="min-w-0">
            <div class="font-semibold truncate">{{ e.name }}</div>
            <div v-if="e.notes" class="text-xs text-muted-foreground italic mt-1">{{ e.notes }}</div>
            <div v-if="e.tags.length" class="flex flex-wrap gap-1.5 mt-2">
              <span
                v-for="t in e.tags"
                :key="t.id"
                class="font-mono text-[10px] tracking-wide text-muted-foreground bg-secondary border border-border rounded px-2 py-1"
              >
                {{ t.name }}
              </span>
            </div>
          </div>
          <Button size="sm" @click="addSetsToLog(e)">Add Sets</Button>
        </div>
      </div>
    </template>

    <template v-else>
      <div v-if="loadingTags" class="flex flex-col gap-6">
        <div class="grid grid-cols-2 gap-2.5">
          <div v-for="i in 4" :key="i" class="aspect-[1.15] rounded-xl bg-card animate-pulse" />
        </div>
      </div>

      <template v-else>
        <section class="flex flex-col gap-2.5">
          <h2 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">Food</h2>
          <div v-if="foodTags.length === 0" class="text-center py-6 text-muted-foreground text-sm italic">
            No food tags yet — create them in the Food Library.
          </div>
          <div v-else class="grid grid-cols-2 gap-3">
            <button
              v-for="t in foodTags"
              :key="t.id"
              type="button"
              class="group relative aspect-[1.15] rounded-2xl border border-border bg-card p-4 flex flex-col justify-end text-left shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)] transition-all hover:-translate-y-0.5 active:scale-[0.97]"
              @click="openFoodTag(t)"
            >
              <span class="absolute top-3.5 right-3.5 h-2 w-2 rounded-full bg-primary/25 transition-colors group-hover:bg-primary" />
              <div class="font-semibold text-base tracking-tight">{{ t.name }}</div>
              <div class="font-mono text-[11px] text-muted-foreground mt-0.5">
                {{ t.recipe_count }} recipe{{ t.recipe_count === 1 ? '' : 's' }}
              </div>
            </button>
          </div>
        </section>

        <section class="flex flex-col gap-2.5">
          <h2 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">Exercises</h2>
          <div v-if="exerciseTags.length === 0" class="text-center py-6 text-muted-foreground text-sm italic">
            No exercise tags yet — create them in the Exercise Library.
          </div>
          <div v-else class="grid grid-cols-2 gap-3">
            <button
              v-for="t in exerciseTags"
              :key="t.id"
              type="button"
              class="group relative aspect-[1.15] rounded-2xl border border-border bg-card p-4 flex flex-col justify-end text-left shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)] transition-all hover:-translate-y-0.5 active:scale-[0.97]"
              @click="openExerciseTag(t)"
            >
              <span class="absolute top-3.5 right-3.5 h-2 w-2 rounded-full bg-primary/25 transition-colors group-hover:bg-primary" />
              <div class="font-semibold text-base tracking-tight">{{ t.name }}</div>
              <div class="font-mono text-[11px] text-muted-foreground mt-0.5">
                {{ t.exercise_count }} exercise{{ t.exercise_count === 1 ? '' : 's' }}
              </div>
            </button>
          </div>
        </section>
      </template>
    </template>
  </div>

  <AddRecipeDrawer
    v-if="showRecipeDrawer && pickedRecipe"
    :user-id="userId"
    :date="today"
    :recipe="pickedRecipe"
    @close="showRecipeDrawer = false"
    @added="onRecipeAdded"
  />

  <CustomRecipeDrawer
    v-if="showCustomDrawer && customizeRecipeId != null"
    :user-id="userId"
    :date="today"
    :prefill-recipe-id="customizeRecipeId"
    @close="showCustomDrawer = false"
    @added="onCustomized"
  />

  <AddSetsDrawer
    v-if="showSetsDrawer && pickedExercise"
    :user-id="userId"
    :date="today"
    :exercise="pickedExercise"
    @close="showSetsDrawer = false"
    @added="onSetsAdded"
  />
</template>
