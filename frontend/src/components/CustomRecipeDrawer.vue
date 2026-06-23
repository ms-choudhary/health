<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { Ingredient, CustomRecipeItem } from '@/lib/types'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Badge from './ui/Badge.vue'
import { Search, X, Plus, Trash2 } from 'lucide-vue-next'

interface DraftItem {
  ingredient_id: number | null
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: string
}

const props = defineProps<{
  userId: number
  date: string
  // When set, the drawer opens pre-filled from this recipe (the "Customize" flow).
  prefillRecipeId?: number
}>()
const emit = defineEmits<{ close: []; added: [] }>()

const name = ref<string>('')
const items = ref<DraftItem[]>([])
const search = ref<string>('')
const searchResults = ref<Ingredient[]>([])
const saving = ref<boolean>(false)
const loadingPrefill = ref<boolean>(false)
const errMsg = ref<string>('')
let searchTimer: number | undefined

const totalCalories = computed<number>(() =>
  items.value.reduce((sum, it) => {
    const q = Number(it.quantity)
    return Number.isFinite(q) && q > 0 ? sum + q * it.calories_per_unit : sum
  }, 0),
)
const totalProtein = computed<number>(() =>
  items.value.reduce((sum, it) => {
    const q = Number(it.quantity)
    return Number.isFinite(q) && q > 0 ? sum + q * it.protein_per_unit : sum
  }, 0),
)

watch(search, (v) => {
  window.clearTimeout(searchTimer)
  const trimmed = v.trim()
  if (!trimmed) {
    searchResults.value = []
    return
  }
  searchTimer = window.setTimeout(async () => {
    searchResults.value = await api.listIngredients(trimmed)
  }, 200)
})

function addItem(ing: Ingredient): void {
  if (items.value.some((i) => i.ingredient_id === ing.id)) return
  items.value.push({
    ingredient_id: ing.id,
    ingredient_name: ing.name,
    ingredient_unit: ing.unit,
    calories_per_unit: ing.calories_per_unit,
    protein_per_unit: ing.protein_per_unit,
    quantity: '1',
  })
  search.value = ''
  searchResults.value = []
}

function removeItem(idx: number): void {
  items.value.splice(idx, 1)
}

async function logRecipe(): Promise<void> {
  const trimmedName = name.value.trim()
  if (!trimmedName) {
    errMsg.value = 'Name required'
    return
  }
  if (items.value.length === 0) {
    errMsg.value = 'Add at least one ingredient'
    return
  }
  const payloadItems: CustomRecipeItem[] = []
  for (const it of items.value) {
    const q = Number(it.quantity)
    if (!Number.isFinite(q) || q <= 0) {
      errMsg.value = `Quantity for ${it.ingredient_name} must be > 0`
      return
    }
    payloadItems.push({
      ingredient_id: it.ingredient_id,
      ingredient_name: it.ingredient_name,
      ingredient_unit: it.ingredient_unit,
      calories_per_unit: it.calories_per_unit,
      protein_per_unit: it.protein_per_unit,
      quantity: q,
    })
  }
  saving.value = true
  errMsg.value = ''
  try {
    await api.logCustomRecipe(props.userId, {
      name: trimmedName,
      date: props.date,
      items: payloadItems,
    })
    emit('added')
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to log recipe'
  } finally {
    saving.value = false
  }
}

async function loadPrefill(recipeId: number): Promise<void> {
  loadingPrefill.value = true
  errMsg.value = ''
  try {
    const recipe = await api.getRecipe(recipeId)
    name.value = recipe.name
    items.value = recipe.ingredients.map((ing) => ({
      ingredient_id: ing.ingredient_id,
      ingredient_name: ing.ingredient_name,
      ingredient_unit: ing.ingredient_unit,
      calories_per_unit: ing.calories_per_unit,
      protein_per_unit: ing.protein_per_unit,
      quantity: String(ing.quantity),
    }))
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load recipe'
  } finally {
    loadingPrefill.value = false
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

onMounted(() => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', onViewportChange)
    vv.addEventListener('scroll', onViewportChange)
    onViewportChange()
  }
  if (props.prefillRecipeId != null) {
    void loadPrefill(props.prefillRecipeId)
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
          <h2 class="font-semibold text-lg">
            {{ prefillRecipeId != null ? 'Customize recipe' : 'Log a custom recipe' }}
          </h2>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div>
          <label class="text-xs text-muted-foreground">Recipe name</label>
          <Input v-model="name" placeholder="e.g. Tuesday lunch" />
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
            Ingredients
          </div>
          <div v-if="items.length === 0" class="text-xs text-muted-foreground">
            Search the ingredient library below to add items.
          </div>
          <div
            v-for="(it, idx) in items"
            :key="`${it.ingredient_id ?? 'x'}-${idx}`"
            class="flex items-center gap-2 rounded-md border border-border p-2"
          >
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium truncate">{{ it.ingredient_name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ it.calories_per_unit }} kcal · {{ it.protein_per_unit }} g / 1 {{ it.ingredient_unit }}
              </div>
            </div>
            <Input
              v-model="it.quantity"
              type="number"
              inputmode="decimal"
              min="0"
              step="0.1"
              class="!w-20"
            />
            <span class="text-xs text-muted-foreground w-10">{{ it.ingredient_unit }}</span>
            <Button variant="ghost" size="icon" @click="removeItem(idx)">
              <Trash2 class="h-4 w-4" />
            </Button>
          </div>
        </div>

        <div class="relative">
          <Input v-model="search" type="search" placeholder="Search ingredient to add…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>
        <div v-if="searchResults.length > 0" class="flex flex-col gap-1 max-h-48 overflow-y-auto">
          <button
            v-for="ing in searchResults"
            :key="ing.id"
            type="button"
            class="text-left p-2 rounded-md border border-border hover:bg-muted transition-colors flex items-center justify-between gap-2"
            @click="addItem(ing)"
          >
            <div class="min-w-0">
              <div class="text-sm font-medium truncate">{{ ing.name }}</div>
              <div class="text-xs text-muted-foreground">
                {{ ing.calories_per_unit }} kcal · {{ ing.protein_per_unit }} g / 1 {{ ing.unit }}
              </div>
            </div>
            <Badge variant="outline"><Plus class="h-3 w-3" /></Badge>
          </button>
        </div>

        <div class="rounded-md bg-muted px-3 py-2 flex items-center justify-between">
          <span class="text-xs text-muted-foreground uppercase tracking-wide">Total</span>
          <span class="font-semibold">
            {{ Math.round(totalCalories) }} kcal · {{ formatNumber(totalProtein, 1) }} g protein
          </span>
        </div>

        <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>

        <div class="flex justify-end gap-2">
          <Button variant="ghost" size="sm" @click="emit('close')">Cancel</Button>
          <Button size="sm" :disabled="saving || loadingPrefill" @click="logRecipe">
            {{ saving ? 'Logging…' : 'Log recipe' }}
          </Button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
