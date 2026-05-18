<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import type { Food, RecipeListItem } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Badge from '@/components/ui/Badge.vue'
import FoodEditor from '@/components/FoodEditor.vue'
import RecipeEditor from '@/components/RecipeEditor.vue'
import { ChevronLeft, Search, Trash2, Plus, Pencil } from 'lucide-vue-next'

type Tab = 'foods' | 'recipes'

const router = useRouter()
const tab = ref<Tab>('foods')
const query = ref<string>('')

const foods = ref<Food[]>([])
const loadingFoods = ref<boolean>(true)

const recipes = ref<RecipeListItem[]>([])
const loadingRecipes = ref<boolean>(true)

const showFoodEditor = ref<boolean>(false)
const editingFood = ref<Food | null>(null)

const showRecipeEditor = ref<boolean>(false)
const editingRecipeId = ref<number | null>(null)

let searchTimer: number | undefined

async function loadFoods(): Promise<void> {
  loadingFoods.value = true
  try {
    foods.value = await api.listFoods(query.value)
  } finally {
    loadingFoods.value = false
  }
}

async function loadRecipes(): Promise<void> {
  loadingRecipes.value = true
  try {
    recipes.value = await api.listRecipes(query.value)
  } finally {
    loadingRecipes.value = false
  }
}

async function loadCurrent(): Promise<void> {
  if (tab.value === 'foods') await loadFoods()
  else await loadRecipes()
}

watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadCurrent, 200)
})

watch(tab, () => {
  query.value = ''
  void loadCurrent()
})

function openNewFood(): void {
  editingFood.value = null
  showFoodEditor.value = true
}

function openEditFood(food: Food): void {
  editingFood.value = food
  showFoodEditor.value = true
}

function onFoodSaved(): void {
  void loadFoods()
}

async function deleteFood(id: number): Promise<void> {
  if (!confirm('Delete this food from the library?')) return
  try {
    await api.deleteFood(id)
    await loadFoods()
  } catch (e) {
    alert(e instanceof Error ? e.message : 'Cannot delete — food may be used by a recipe.')
  }
}

function openNewRecipe(): void {
  editingRecipeId.value = null
  showRecipeEditor.value = true
}

function openEditRecipe(id: number): void {
  editingRecipeId.value = id
  showRecipeEditor.value = true
}

async function deleteRecipe(id: number): Promise<void> {
  if (!confirm('Delete this recipe? Past logs are unaffected.')) return
  await api.deleteRecipe(id)
  await loadRecipes()
}

function onRecipeSaved(): void {
  void loadRecipes()
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
      <h1 class="text-xl font-bold">Library</h1>
    </header>

    <div class="flex gap-2">
      <Button :variant="tab === 'foods' ? 'default' : 'outline'" size="sm" @click="tab = 'foods'">
        Foods
      </Button>
      <Button :variant="tab === 'recipes' ? 'default' : 'outline'" size="sm" @click="tab = 'recipes'">
        Recipes
      </Button>
    </div>

    <div class="relative">
      <Input v-model="query" type="search" :placeholder="tab === 'foods' ? 'Search food…' : 'Search recipes…'" />
      <Search class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
    </div>

    <template v-if="tab === 'foods'">
      <div v-if="loadingFoods" class="flex flex-col gap-2">
        <div v-for="i in 4" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
      </div>

      <div v-else-if="foods.length === 0" class="text-center py-10 text-muted-foreground text-sm">
        <template v-if="query">No food matches "{{ query }}".</template>
        <template v-else>No foods yet — tap "Add food" below.</template>
      </div>

      <div v-else class="flex flex-col gap-2">
        <Card v-for="f in foods" :key="f.id">
          <div class="p-3 flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="font-medium truncate">{{ f.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ f.calories_per_unit }} kcal / 1 {{ f.unit }}
              </div>
            </div>
            <Badge variant="secondary">{{ f.unit }}</Badge>
            <Button variant="ghost" size="icon" @click="openEditFood(f)">
              <Pencil class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" @click="deleteFood(f.id)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </Card>
      </div>

      <Button class="mt-2" @click="openNewFood">
        <Plus class="h-4 w-4" />
        Add food
      </Button>
    </template>

    <template v-else>
      <div v-if="loadingRecipes" class="flex flex-col gap-2">
        <div v-for="i in 4" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
      </div>

      <div v-else-if="recipes.length === 0" class="text-center py-10 text-muted-foreground text-sm">
        <template v-if="query">No recipe matches "{{ query }}".</template>
        <template v-else>No recipes yet — tap "New recipe" below.</template>
      </div>

      <div v-else class="flex flex-col gap-2">
        <Card v-for="r in recipes" :key="r.id">
          <div class="p-3 flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="font-medium truncate">{{ r.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ Math.round(r.total_calories) }} kcal / serving
              </div>
            </div>
            <Badge variant="secondary">Recipe</Badge>
            <Button variant="ghost" size="icon" @click="openEditRecipe(r.id)">
              <Pencil class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" @click="deleteRecipe(r.id)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </Card>
      </div>

      <Button class="mt-2" @click="openNewRecipe">
        <Plus class="h-4 w-4" />
        New recipe
      </Button>
    </template>
  </div>

  <FoodEditor
    v-model:open="showFoodEditor"
    :food="editingFood"
    @saved="onFoodSaved"
  />

  <RecipeEditor
    v-model:open="showRecipeEditor"
    :recipe-id="editingRecipeId"
    @saved="onRecipeSaved"
  />
</template>
