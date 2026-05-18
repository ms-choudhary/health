<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { api } from '@/lib/api'
import type {
  Food,
  RecipeWithIngredients,
  RecipeIngredientInput,
} from '@/lib/types'
import Dialog from '@/components/ui/Dialog.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Badge from '@/components/ui/Badge.vue'
import { Plus, Trash2, Search } from 'lucide-vue-next'

interface DraftIngredient {
  food_id: number
  food_name: string
  food_unit: string
  calories_per_unit: number
  quantity: string
}

const props = defineProps<{ open: boolean; recipeId: number | null }>()
const emit = defineEmits<{
  'update:open': [v: boolean]
  saved: []
}>()

const name = ref<string>('')
const ingredients = ref<DraftIngredient[]>([])
const search = ref<string>('')
const searchResults = ref<Food[]>([])
const saving = ref<boolean>(false)
const loading = ref<boolean>(false)
const errMsg = ref<string>('')
let searchTimer: number | undefined

const isEdit = computed<boolean>(() => props.recipeId != null)

const totalCalories = computed<number>(() => {
  let total = 0
  for (const ing of ingredients.value) {
    const qty = Number(ing.quantity)
    if (Number.isFinite(qty) && qty > 0) {
      total += qty * ing.calories_per_unit
    }
  }
  return total
})

function close(): void {
  emit('update:open', false)
}

function reset(): void {
  name.value = ''
  ingredients.value = []
  search.value = ''
  searchResults.value = []
  errMsg.value = ''
}

async function loadForEdit(id: number): Promise<void> {
  loading.value = true
  try {
    const recipe: RecipeWithIngredients = await api.getRecipe(id)
    name.value = recipe.name
    ingredients.value = recipe.ingredients.map((ing) => ({
      food_id: ing.food_id,
      food_name: ing.food_name,
      food_unit: ing.food_unit,
      calories_per_unit: ing.calories_per_unit,
      quantity: String(ing.quantity),
    }))
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load recipe'
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    reset()
    if (props.recipeId != null) {
      await loadForEdit(props.recipeId)
    }
  },
)

watch(search, (v) => {
  window.clearTimeout(searchTimer)
  const trimmed = v.trim()
  if (!trimmed) {
    searchResults.value = []
    return
  }
  searchTimer = window.setTimeout(async () => {
    searchResults.value = await api.listFoods(trimmed)
  }, 200)
})

function addIngredient(food: Food): void {
  if (ingredients.value.some((i) => i.food_id === food.id)) return
  ingredients.value.push({
    food_id: food.id,
    food_name: food.name,
    food_unit: food.unit,
    calories_per_unit: food.calories_per_unit,
    quantity: '1',
  })
  search.value = ''
  searchResults.value = []
}

function removeIngredient(foodId: number): void {
  ingredients.value = ingredients.value.filter((i) => i.food_id !== foodId)
}

async function save(): Promise<void> {
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errMsg.value = 'Name required'
    return
  }
  if (ingredients.value.length === 0) {
    errMsg.value = 'Add at least one ingredient'
    return
  }
  const payload: { name: string; ingredients: RecipeIngredientInput[] } = {
    name: trimmedName,
    ingredients: [],
  }
  for (const ing of ingredients.value) {
    const qty = Number(ing.quantity)
    if (!Number.isFinite(qty) || qty <= 0) {
      errMsg.value = `Quantity for ${ing.food_name} must be > 0`
      return
    }
    payload.ingredients.push({ food_id: ing.food_id, quantity: qty })
  }
  saving.value = true
  errMsg.value = ''
  try {
    if (props.recipeId != null) {
      await api.updateRecipe(props.recipeId, payload)
    } else {
      await api.createRecipe(payload)
    }
    emit('saved')
    close()
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to save recipe'
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  if (props.open && props.recipeId != null) {
    void loadForEdit(props.recipeId)
  }
})
</script>

<template>
  <Dialog :open="open" :title="isEdit ? 'Edit recipe' : 'New recipe'" @update:open="(v) => emit('update:open', v)">
    <div v-if="loading" class="py-6 text-center text-sm text-muted-foreground">Loading…</div>
    <div v-else class="flex flex-col gap-3">
      <div>
        <label class="text-xs text-muted-foreground">Recipe name</label>
        <Input v-model="name" placeholder="e.g. Mango Shake" />
      </div>

      <div class="flex flex-col gap-2">
        <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
          Ingredients
        </div>
        <div v-if="ingredients.length === 0" class="text-xs text-muted-foreground">
          Add ingredients by searching the food library below.
        </div>
        <div
          v-for="ing in ingredients"
          :key="ing.food_id"
          class="flex items-center gap-2 rounded-md border border-border p-2"
        >
          <div class="flex-1 min-w-0">
            <div class="text-sm font-medium truncate">{{ ing.food_name }}</div>
            <div class="text-xs text-muted-foreground">
              {{ ing.calories_per_unit }} kcal / 1 {{ ing.food_unit }}
            </div>
          </div>
          <Input
            v-model="ing.quantity"
            type="number"
            inputmode="decimal"
            min="0"
            step="0.1"
            class="!w-20"
          />
          <span class="text-xs text-muted-foreground w-10">{{ ing.food_unit }}</span>
          <Button variant="ghost" size="icon" @click="removeIngredient(ing.food_id)">
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <div class="flex flex-col gap-2">
        <div class="relative">
          <Input v-model="search" type="search" placeholder="Search food to add…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>
        <div v-if="searchResults.length > 0" class="flex flex-col gap-1 max-h-48 overflow-y-auto">
          <button
            v-for="food in searchResults"
            :key="food.id"
            type="button"
            class="text-left p-2 rounded-md border border-border hover:bg-muted transition-colors flex items-center justify-between gap-2"
            @click="addIngredient(food)"
          >
            <div class="min-w-0">
              <div class="text-sm font-medium truncate">{{ food.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ food.calories_per_unit }} kcal / 1 {{ food.unit }}
              </div>
            </div>
            <Badge variant="outline">
              <Plus class="h-3 w-3" />
            </Badge>
          </button>
        </div>
      </div>

      <div class="rounded-md bg-muted px-3 py-2 flex items-center justify-between">
        <span class="text-xs text-muted-foreground uppercase tracking-wide">Total per serving</span>
        <span class="font-semibold">{{ Math.round(totalCalories) }} kcal</span>
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
