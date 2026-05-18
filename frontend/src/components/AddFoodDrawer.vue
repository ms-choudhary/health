<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { api } from '@/lib/api'
import type {
  Food,
  RecentFood,
  RecipeListItem,
  AddLogPayload,
  Pickable,
} from '@/lib/types'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Badge from './ui/Badge.vue'
import { Search, X } from 'lucide-vue-next'

interface PickedFood {
  kind: 'food'
  food_id: number | null
  food_name: string
  food_unit: string
  calories_per_unit: number
  quantity: string
}

interface PickedRecipe {
  kind: 'recipe'
  recipe_id: number
  recipe_name: string
  total_calories: number
  scale: string
}

type Picked = PickedFood | PickedRecipe

const props = defineProps<{ userId: number; date: string }>()
const emit = defineEmits<{
  close: []
  added: [payload: AddLogPayload | { recipe_id: number }]
}>()

const query = ref<string>('')
const recent = ref<RecentFood[]>([])
const results = ref<Pickable[]>([])
const picked = ref<Picked | null>(null)
const saving = ref<boolean>(false)
const errMsg = ref<string>('')

let searchTimer: number | undefined

const previewCalories = computed<number>(() => {
  if (!picked.value) return 0
  const n = Number(picked.value.kind === 'food' ? picked.value.quantity : picked.value.scale)
  if (!Number.isFinite(n) || n <= 0) return 0
  if (picked.value.kind === 'food') return n * picked.value.calories_per_unit
  return n * picked.value.total_calories
})

async function loadRecent(): Promise<void> {
  try {
    recent.value = await api.recentFoods(props.userId)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load recent foods'
  }
}

watch(query, (v) => {
  window.clearTimeout(searchTimer)
  const trimmed = v.trim()
  if (!trimmed) {
    results.value = []
    return
  }
  searchTimer = window.setTimeout(async () => {
    const [foods, recipes] = await Promise.all([
      api.listFoods(trimmed),
      api.listRecipes(trimmed),
    ])
    const mapped: Pickable[] = [
      ...recipes.map((r: RecipeListItem): Pickable => ({ kind: 'recipe', recipe: r })),
      ...foods.map((f: Food): Pickable => ({ kind: 'food', food: f })),
    ]
    results.value = mapped
  }, 200)
})

function pickRecent(item: RecentFood): void {
  picked.value = {
    kind: 'food',
    food_id: item.food_id,
    food_name: item.food_name,
    food_unit: item.food_unit,
    calories_per_unit: item.calories_per_unit,
    quantity: String(item.last_quantity),
  }
}

function pickLibraryFood(food: Food): void {
  picked.value = {
    kind: 'food',
    food_id: food.id,
    food_name: food.name,
    food_unit: food.unit,
    calories_per_unit: food.calories_per_unit,
    quantity: '1',
  }
}

function pickLibraryRecipe(recipe: RecipeListItem): void {
  picked.value = {
    kind: 'recipe',
    recipe_id: recipe.id,
    recipe_name: recipe.name,
    total_calories: recipe.total_calories,
    scale: '1',
  }
}

function clearPicked(): void {
  picked.value = null
  errMsg.value = ''
}

async function confirm(): Promise<void> {
  if (!picked.value) return
  if (picked.value.kind === 'food') {
    const n = Number(picked.value.quantity)
    if (!Number.isFinite(n) || n <= 0) {
      errMsg.value = 'Quantity must be a positive number'
      return
    }
    saving.value = true
    errMsg.value = ''
    const payload: AddLogPayload = {
      food_id: picked.value.food_id,
      food_name: picked.value.food_name,
      food_unit: picked.value.food_unit,
      calories_per_unit: picked.value.calories_per_unit,
      quantity: n,
      date: props.date,
    }
    try {
      await api.addLog(props.userId, payload)
      emit('added', payload)
    } catch (e) {
      errMsg.value = e instanceof Error ? e.message : 'Failed to add entry'
    } finally {
      saving.value = false
    }
    return
  }
  const scale = Number(picked.value.scale)
  if (!Number.isFinite(scale) || scale <= 0) {
    errMsg.value = 'Servings must be a positive number'
    return
  }
  saving.value = true
  errMsg.value = ''
  const recipeId = picked.value.recipe_id
  try {
    await api.logRecipe(props.userId, {
      recipe_id: recipeId,
      scale,
      date: props.date,
    })
    emit('added', { recipe_id: recipeId })
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to log recipe'
  } finally {
    saving.value = false
  }
}

function onKey(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  void loadRecent()
})

onUnmounted(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <Teleport to="body">
    <div class="fixed inset-0 z-50 flex items-end justify-center" @click.self="emit('close')">
      <div class="absolute inset-0 bg-black/50" />
      <div
        class="relative w-full max-w-lg bg-card text-card-foreground border-t border-border rounded-t-2xl p-4 pb-8 flex flex-col gap-3 max-h-[85vh] overflow-y-auto"
      >
        <div class="mx-auto h-1 w-10 rounded-full bg-border" />

        <div class="flex items-center justify-between">
          <h2 class="font-semibold text-lg">Add to log</h2>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div class="relative">
          <Input v-model="query" type="search" placeholder="Search foods and recipes…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>

        <div v-if="picked" class="rounded-lg bg-muted p-3 flex flex-col gap-2">
          <template v-if="picked.kind === 'food'">
            <div>
              <div class="font-medium">{{ picked.food_name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ picked.calories_per_unit }} kcal / 1 {{ picked.food_unit }}
              </div>
            </div>
            <div class="flex items-center gap-2">
              <Input
                v-model="picked.quantity"
                type="number"
                inputmode="decimal"
                min="0.1"
                step="0.1"
                placeholder="Quantity"
                class="!w-28"
              />
              <span class="text-sm text-muted-foreground">{{ picked.food_unit }}</span>
              <span v-if="previewCalories > 0" class="text-sm ml-auto">
                ≈ {{ Math.round(previewCalories) }} kcal
              </span>
            </div>
          </template>
          <template v-else>
            <div>
              <div class="font-medium flex items-center gap-2">
                <span>{{ picked.recipe_name }}</span>
                <Badge variant="secondary">Recipe</Badge>
              </div>
              <div class="text-xs text-muted-foreground">
                {{ Math.round(picked.total_calories) }} kcal / serving
              </div>
            </div>
            <div class="flex items-center gap-2">
              <Input
                v-model="picked.scale"
                type="number"
                inputmode="decimal"
                min="0.1"
                step="0.5"
                placeholder="Servings"
                class="!w-28"
              />
              <span class="text-sm text-muted-foreground">serving(s)</span>
              <span v-if="previewCalories > 0" class="text-sm ml-auto">
                ≈ {{ Math.round(previewCalories) }} kcal
              </span>
            </div>
          </template>
          <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>
          <div class="flex gap-2 justify-end">
            <Button variant="ghost" size="sm" @click="clearPicked">Cancel</Button>
            <Button size="sm" :disabled="saving" @click="confirm">
              {{ saving ? 'Adding…' : 'Add to log' }}
            </Button>
          </div>
        </div>

        <template v-else>
          <div v-if="!query.trim()" class="flex flex-col gap-2">
            <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              Recent
            </div>
            <div
              v-if="recent.length === 0"
              class="text-sm text-muted-foreground py-4 text-center"
            >
              No recent foods — search above to add one.
            </div>
            <button
              v-for="item in recent"
              :key="item.food_name"
              type="button"
              class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
              @click="pickRecent(item)"
            >
              <div class="min-w-0">
                <div class="font-medium truncate">{{ item.food_name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ item.last_quantity }} {{ item.food_unit }} ·
                  {{ Math.round(item.last_quantity * item.calories_per_unit) }} kcal last time
                </div>
              </div>
              <Badge variant="secondary">
                {{ Math.round(item.last_quantity * item.calories_per_unit) }}
              </Badge>
            </button>
          </div>

          <div v-else class="flex flex-col gap-2">
            <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              Library
            </div>
            <div
              v-if="results.length === 0"
              class="text-sm text-muted-foreground py-4 text-center"
            >
              No matches. Add foods/recipes in the
              <RouterLink to="/library" class="underline">Library</RouterLink>
              first.
            </div>
            <template v-for="item in results">
              <button
                v-if="item.kind === 'recipe'"
                :key="`r-${item.recipe.id}`"
                type="button"
                class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
                @click="pickLibraryRecipe(item.recipe)"
              >
                <div class="min-w-0">
                  <div class="font-medium truncate">{{ item.recipe.name }}</div>
                  <div class="text-xs text-muted-foreground">
                    {{ Math.round(item.recipe.total_calories) }} kcal / serving
                  </div>
                </div>
                <Badge variant="secondary">Recipe</Badge>
              </button>
              <button
                v-else
                :key="`f-${item.food.id}`"
                type="button"
                class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
                @click="pickLibraryFood(item.food)"
              >
                <div class="min-w-0">
                  <div class="font-medium truncate">{{ item.food.name }}</div>
                  <div class="text-xs text-muted-foreground">
                    {{ item.food.calories_per_unit }} kcal / 1 {{ item.food.unit }}
                  </div>
                </div>
                <Badge variant="outline">{{ item.food.unit }}</Badge>
              </button>
            </template>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>
