<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { format } from 'date-fns'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { FoodTagWithCount, RecipeListItem } from '@/lib/types'
import Button from '@/components/ui/Button.vue'
import AddRecipeDrawer from '@/components/AddRecipeDrawer.vue'

const props = defineProps<{ userId: number }>()

const today = format(new Date(), 'yyyy-MM-dd')
const dayName = format(new Date(), 'EEEE')
const dateLabel = format(new Date(), 'MMMM d, yyyy')

const tags = ref<FoodTagWithCount[]>([])
const loadingTags = ref<boolean>(true)

const selectedTag = ref<FoodTagWithCount | null>(null)
const recipes = ref<RecipeListItem[]>([])
const loadingRecipes = ref<boolean>(false)

const showDrawer = ref<boolean>(false)
const pickedRecipe = ref<RecipeListItem | null>(null)

async function loadTags(): Promise<void> {
  loadingTags.value = true
  try {
    tags.value = await api.listFoodTags()
  } finally {
    loadingTags.value = false
  }
}

async function openTag(t: FoodTagWithCount): Promise<void> {
  selectedTag.value = t
  loadingRecipes.value = true
  try {
    recipes.value = await api.recipesByFoodTag(t.id)
  } finally {
    loadingRecipes.value = false
  }
}

function addToLog(recipe: RecipeListItem): void {
  pickedRecipe.value = recipe
  showDrawer.value = true
}

function onAdded(): void {
  showDrawer.value = false
  pickedRecipe.value = null
}

onMounted(loadTags)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div>
      <div class="font-mono text-xs uppercase tracking-widest text-primary">{{ dayName }}</div>
      <div class="text-3xl font-bold tracking-tight">{{ dateLabel }}</div>
    </div>

    <template v-if="selectedTag">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-bold lowercase">{{ selectedTag.name }}</h2>
        <Button variant="secondary" size="sm" @click="selectedTag = null">← Back</Button>
      </div>
      <div v-if="loadingRecipes" class="flex flex-col gap-2">
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
          <Button size="sm" @click="addToLog(r)">Add to log</Button>
        </div>
      </div>
    </template>

    <template v-else>
      <div v-if="loadingTags" class="grid grid-cols-2 gap-2.5">
        <div v-for="i in 6" :key="i" class="aspect-[1.15] rounded-xl bg-card animate-pulse" />
      </div>
      <div v-else-if="tags.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
        No tags yet — create them in the Library’s “Tags” tab.
      </div>
      <div v-else class="grid grid-cols-2 gap-3">
        <button
          v-for="t in tags"
          :key="t.id"
          type="button"
          class="group relative aspect-[1.15] rounded-2xl border border-border bg-card p-4 flex flex-col justify-end text-left shadow-[0_1px_2px_rgb(16_24_40/0.04),0_4px_12px_rgb(16_24_40/0.05)] transition-all hover:-translate-y-0.5 active:scale-[0.97]"
          @click="openTag(t)"
        >
          <span class="absolute top-3.5 right-3.5 h-2 w-2 rounded-full bg-primary/25 transition-colors group-hover:bg-primary" />
          <div class="font-semibold text-base tracking-tight">{{ t.name }}</div>
          <div class="font-mono text-[11px] text-muted-foreground mt-0.5">
            {{ t.recipe_count }} recipe{{ t.recipe_count === 1 ? '' : 's' }}
          </div>
        </button>
      </div>
    </template>
  </div>

  <AddRecipeDrawer
    v-if="showDrawer"
    :user-id="userId"
    :date="today"
    :initial-recipe="pickedRecipe"
    @close="showDrawer = false"
    @added="onAdded"
  />
</template>
