<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import type { Ingredient, RecipeListItem, FoodTagWithCount } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Badge from '@/components/ui/Badge.vue'
import IngredientEditor from '@/components/IngredientEditor.vue'
import RecipeEditor from '@/components/RecipeEditor.vue'
import FoodTagEditor from '@/components/FoodTagEditor.vue'
import { ChevronLeft, Search, Trash2, Plus, Pencil } from 'lucide-vue-next'

type Tab = 'ingredients' | 'recipes' | 'tags'

const router = useRouter()
const tab = ref<Tab>('ingredients')
const query = ref<string>('')

const ingredients = ref<Ingredient[]>([])
const loadingIngredients = ref<boolean>(true)

const recipes = ref<RecipeListItem[]>([])
const loadingRecipes = ref<boolean>(true)

const foodTags = ref<FoodTagWithCount[]>([])
const loadingFoodTags = ref<boolean>(true)
const showFoodTagEditor = ref<boolean>(false)

const selectedFoodTag = ref<FoodTagWithCount | null>(null)
const taggedRecipes = ref<RecipeListItem[]>([])
const loadingTaggedRecipes = ref<boolean>(false)

const showIngredientEditor = ref<boolean>(false)
const editingIngredient = ref<Ingredient | null>(null)

const showRecipeEditor = ref<boolean>(false)
const editingRecipeId = ref<number | null>(null)

let searchTimer: number | undefined

async function loadIngredients(): Promise<void> {
  loadingIngredients.value = true
  try {
    ingredients.value = await api.listIngredients(query.value)
  } finally {
    loadingIngredients.value = false
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

async function loadFoodTags(): Promise<void> {
  loadingFoodTags.value = true
  try {
    foodTags.value = await api.listFoodTags()
  } finally {
    loadingFoodTags.value = false
  }
}

function openNewFoodTag(): void {
  showFoodTagEditor.value = true
}

function onFoodTagSaved(): void {
  void loadFoodTags()
}

async function deleteFoodTag(id: number): Promise<void> {
  if (!confirm('Delete this tag? It will be removed from all recipes.')) return
  await api.deleteFoodTag(id)
  if (selectedFoodTag.value?.id === id) selectedFoodTag.value = null
  await loadFoodTags()
}

async function openFoodTag(t: FoodTagWithCount): Promise<void> {
  selectedFoodTag.value = t
  loadingTaggedRecipes.value = true
  try {
    taggedRecipes.value = await api.recipesByFoodTag(t.id)
  } finally {
    loadingTaggedRecipes.value = false
  }
}

async function loadCurrent(): Promise<void> {
  if (tab.value === 'ingredients') await loadIngredients()
  else if (tab.value === 'recipes') await loadRecipes()
  else await loadFoodTags()
}

watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadCurrent, 200)
})

watch(tab, () => {
  query.value = ''
  selectedFoodTag.value = null
  void loadCurrent()
})

function openNewIngredient(): void {
  editingIngredient.value = null
  showIngredientEditor.value = true
}

function openEditIngredient(ingredient: Ingredient): void {
  editingIngredient.value = ingredient
  showIngredientEditor.value = true
}

function onIngredientSaved(): void {
  void loadIngredients()
}

async function deleteIngredient(id: number): Promise<void> {
  if (!confirm('Delete this ingredient from the library?')) return
  try {
    await api.deleteIngredient(id)
    await loadIngredients()
  } catch (e) {
    alert(e instanceof Error ? e.message : 'Cannot delete — ingredient may be used by a recipe.')
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
      <h1 class="text-xl font-bold">Food Library</h1>
    </header>

    <div class="flex gap-2">
      <Button :variant="tab === 'ingredients' ? 'default' : 'outline'" size="sm" @click="tab = 'ingredients'">
        Ingredients
      </Button>
      <Button :variant="tab === 'recipes' ? 'default' : 'outline'" size="sm" @click="tab = 'recipes'">
        Recipes
      </Button>
      <Button :variant="tab === 'tags' ? 'default' : 'outline'" size="sm" @click="tab = 'tags'">
        Tags
      </Button>
    </div>

    <div v-if="tab !== 'tags'" class="relative">
      <Input v-model="query" type="search" :placeholder="tab === 'ingredients' ? 'Search ingredients…' : 'Search recipes…'" />
      <Search class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
    </div>

    <template v-if="tab === 'ingredients'">
      <div v-if="loadingIngredients" class="flex flex-col gap-2">
        <div v-for="i in 4" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
      </div>

      <div v-else-if="ingredients.length === 0" class="text-center py-10 text-muted-foreground text-sm">
        <template v-if="query">No ingredient matches "{{ query }}".</template>
        <template v-else>No ingredients yet — tap "Add ingredient" below.</template>
      </div>

      <div v-else class="flex flex-col gap-2">
        <Card v-for="f in ingredients" :key="f.id">
          <div class="p-3 flex items-center gap-3">
            <div class="flex-1 min-w-0">
              <div class="font-medium truncate">{{ f.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ f.calories_per_unit }} kcal · {{ f.protein_per_unit }} g protein / 1 {{ f.unit }}
              </div>
            </div>
            <Badge variant="secondary">{{ f.unit }}</Badge>
            <Button variant="ghost" size="icon" @click="openEditIngredient(f)">
              <Pencil class="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" @click="deleteIngredient(f.id)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </Card>
      </div>

      <Button class="mt-2" @click="openNewIngredient">
        <Plus class="h-4 w-4" />
        Add ingredient
      </Button>
    </template>

    <template v-else-if="tab === 'recipes'">
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
                {{ Math.round(r.total_calories) }} kcal · {{ Math.round(r.total_protein) }} g protein / serving
              </div>
              <div v-if="r.food_tags.length" class="mt-1 flex flex-wrap gap-1">
                <Badge v-for="t in r.food_tags" :key="t.id" variant="outline">{{ t.name }}</Badge>
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

    <template v-else>
      <template v-if="selectedFoodTag">
        <Button variant="ghost" size="sm" class="self-start" @click="selectedFoodTag = null">
          ← All tags
        </Button>
        <div class="text-sm font-medium">Recipes tagged “{{ selectedFoodTag.name }}”</div>
        <div v-if="loadingTaggedRecipes" class="flex flex-col gap-2">
          <div v-for="i in 3" :key="i" class="h-14 rounded-lg bg-muted animate-pulse" />
        </div>
        <div v-else-if="taggedRecipes.length === 0" class="text-center py-10 text-muted-foreground text-sm">
          No recipes have this tag yet.
        </div>
        <div v-else class="flex flex-col gap-2">
          <Card v-for="r in taggedRecipes" :key="r.id">
            <div class="p-3">
              <div class="font-medium truncate">{{ r.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ Math.round(r.total_calories) }} kcal · {{ Math.round(r.total_protein) }} g protein / serving
              </div>
            </div>
          </Card>
        </div>
      </template>

      <template v-else>
        <div v-if="loadingFoodTags" class="flex flex-col gap-2">
          <div v-for="i in 4" :key="i" class="h-12 rounded-lg bg-muted animate-pulse" />
        </div>
        <div v-else-if="foodTags.length === 0" class="text-center py-10 text-muted-foreground text-sm">
          No tags yet — tap "Add tag" below.
        </div>
        <div v-else class="flex flex-col gap-2">
          <Card v-for="t in foodTags" :key="t.id">
            <div class="p-3 flex items-center gap-3">
              <button class="flex-1 min-w-0 text-left" @click="openFoodTag(t)">
                <div class="font-medium truncate">{{ t.name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ t.recipe_count }} recipe{{ t.recipe_count === 1 ? '' : 's' }}
                </div>
              </button>
              <Button variant="ghost" size="icon" @click="deleteFoodTag(t.id)">
                <Trash2 class="h-4 w-4" />
              </Button>
            </div>
          </Card>
        </div>

        <Button class="mt-2" @click="openNewFoodTag">
          <Plus class="h-4 w-4" />
          Add tag
        </Button>
      </template>
    </template>
  </div>

  <IngredientEditor
    v-model:open="showIngredientEditor"
    :ingredient="editingIngredient"
    @saved="onIngredientSaved"
  />

  <RecipeEditor
    v-model:open="showRecipeEditor"
    :recipe-id="editingRecipeId"
    @saved="onRecipeSaved"
  />

  <FoodTagEditor
    v-model:open="showFoodTagEditor"
    @saved="onFoodTagSaved"
  />
</template>
