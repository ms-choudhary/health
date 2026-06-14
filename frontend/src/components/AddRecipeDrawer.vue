<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { RecentItem, RecipeListItem } from '@/lib/types'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Badge from './ui/Badge.vue'
import { Search, X } from 'lucide-vue-next'

interface Picked {
  recipe_id: number
  recipe_name: string
  total_calories: number
  total_protein: number
  scale: string
}

const props = defineProps<{ userId: number; date: string }>()
const emit = defineEmits<{
  close: []
  added: [payload: { recipe_id: number }]
}>()

const query = ref<string>('')
const recent = ref<RecentItem[]>([])
const results = ref<RecipeListItem[]>([])
const picked = ref<Picked | null>(null)
const saving = ref<boolean>(false)
const errMsg = ref<string>('')

let searchTimer: number | undefined

const previewCalories = computed<number>(() => {
  if (!picked.value) return 0
  const n = Number(picked.value.scale)
  if (!Number.isFinite(n) || n <= 0) return 0
  return n * picked.value.total_calories
})

const previewProtein = computed<number>(() => {
  if (!picked.value) return 0
  const n = Number(picked.value.scale)
  if (!Number.isFinite(n) || n <= 0) return 0
  return n * picked.value.total_protein
})

async function loadRecent(): Promise<void> {
  try {
    recent.value = await api.recentRecipes(props.userId)
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to load recent recipes'
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
    results.value = await api.listRecipes(trimmed)
  }, 200)
})

function pickRecent(item: RecentItem): void {
  picked.value = {
    recipe_id: item.recipe_id,
    recipe_name: item.recipe_name,
    total_calories: item.total_calories,
    total_protein: item.total_protein,
    scale: String(item.last_servings),
  }
}

function pickRecipe(recipe: RecipeListItem): void {
  picked.value = {
    recipe_id: recipe.id,
    recipe_name: recipe.name,
    total_calories: recipe.total_calories,
    total_protein: recipe.total_protein,
    scale: '1',
  }
}

function clearPicked(): void {
  picked.value = null
  errMsg.value = ''
}

async function confirm(): Promise<void> {
  if (!picked.value) return
  const servings = Number(picked.value.scale)
  if (!Number.isFinite(servings) || servings <= 0) {
    errMsg.value = 'Servings must be a positive number'
    return
  }
  saving.value = true
  errMsg.value = ''
  const recipeId = picked.value.recipe_id
  try {
    await api.logRecipe(props.userId, {
      recipe_id: recipeId,
      servings,
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

// Track the visual viewport so the drawer stays pinned above the on-screen
// keyboard. On iOS the keyboard shrinks the visual viewport but not the layout
// viewport, so a short, bottom-anchored drawer would otherwise hide behind it.
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
  void loadRecent()
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
          <h2 class="font-semibold text-lg">Add to log</h2>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div class="relative">
          <Input v-model="query" type="search" placeholder="Search recipes…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>

        <div v-if="picked" class="rounded-lg bg-muted p-3 flex flex-col gap-2">
          <div>
            <div class="font-medium flex items-center gap-2">
              <span>{{ picked.recipe_name }}</span>
              <Badge variant="secondary">Recipe</Badge>
            </div>
            <div class="text-xs text-muted-foreground">
              {{ Math.round(picked.total_calories) }} kcal · {{ formatNumber(picked.total_protein, 1) }} g protein / serving
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
              ≈ {{ Math.round(previewCalories) }} kcal · {{ formatNumber(previewProtein, 1) }} g
            </span>
          </div>
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
              No recent recipes — search above to add one.
            </div>
            <button
              v-for="item in recent"
              :key="`r-${item.recipe_id}`"
              type="button"
              class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
              @click="pickRecent(item)"
            >
              <div class="min-w-0">
                <div class="font-medium truncate">{{ item.recipe_name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ formatNumber(item.last_servings, item.last_servings % 1 ? 1 : 0) }}
                  serving{{ item.last_servings === 1 ? '' : 's' }} last time ·
                  {{ Math.round(item.last_servings * item.total_calories) }} kcal
                </div>
              </div>
              <Badge variant="secondary">Recipe</Badge>
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
              No recipes match. Create one in the
              <RouterLink to="/library" class="underline">Library</RouterLink>
              first.
            </div>
            <button
              v-for="recipe in results"
              :key="`r-${recipe.id}`"
              type="button"
              class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
              @click="pickRecipe(recipe)"
            >
              <div class="min-w-0">
                <div class="font-medium truncate">{{ recipe.name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ Math.round(recipe.total_calories) }} kcal · {{ formatNumber(recipe.total_protein, 1) }} g protein / serving
                </div>
              </div>
              <Badge variant="secondary">Recipe</Badge>
            </button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>
