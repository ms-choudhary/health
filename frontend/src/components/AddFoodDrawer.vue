<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { api } from '@/lib/api'
import type { Food, RecentFood, AddLogPayload } from '@/lib/types'
import Button from './ui/Button.vue'
import Input from './ui/Input.vue'
import Badge from './ui/Badge.vue'
import { Search, X } from 'lucide-vue-next'

interface PickedFood {
  food_id: number | null
  food_name: string
  food_unit: string
  calories_per_unit: number
  last_quantity: number
}

const props = defineProps<{ userId: number; date: string }>()
const emit = defineEmits<{
  close: []
  added: [entry: AddLogPayload]
}>()

const query = ref('')
const recent = ref<RecentFood[]>([])
const results = ref<Food[]>([])
const picked = ref<PickedFood | null>(null)
const qty = ref<string>('')
const saving = ref(false)
const errMsg = ref('')

let searchTimer: number | undefined

async function loadRecent() {
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
    results.value = await api.listFoods(trimmed)
  }, 200)
})

function pickRecent(item: RecentFood) {
  picked.value = {
    food_id: item.food_id,
    food_name: item.food_name,
    food_unit: item.food_unit,
    calories_per_unit: item.calories_per_unit,
    last_quantity: item.last_quantity,
  }
  qty.value = String(item.last_quantity)
}

function pickLibrary(food: Food) {
  picked.value = {
    food_id: food.id,
    food_name: food.name,
    food_unit: food.unit,
    calories_per_unit: food.calories_per_unit,
    last_quantity: 1,
  }
  qty.value = '1'
}

function clearPicked() {
  picked.value = null
  qty.value = ''
  errMsg.value = ''
}

async function confirm() {
  if (!picked.value) return
  const n = Number(qty.value)
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
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  document.body.style.overflow = 'hidden'
  window.addEventListener('keydown', onKey)
  loadRecent()
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
          <h2 class="font-semibold text-lg">Add food</h2>
          <Button variant="ghost" size="icon" @click="emit('close')">
            <X class="h-4 w-4" />
          </Button>
        </div>

        <div class="relative">
          <Input v-model="query" type="search" placeholder="Search food library…" />
          <Search
            class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none"
          />
        </div>

        <div
          v-if="picked"
          class="rounded-lg bg-muted p-3 flex flex-col gap-2"
        >
          <div>
            <div class="font-medium">{{ picked.food_name }}</div>
            <div class="text-xs text-muted-foreground">
              {{ picked.calories_per_unit }} kcal / 1 {{ picked.food_unit }}
            </div>
          </div>
          <div class="flex items-center gap-2">
            <Input
              v-model="qty"
              type="number"
              inputmode="decimal"
              min="0.1"
              step="0.1"
              placeholder="Quantity"
              class="!w-28"
            />
            <span class="text-sm text-muted-foreground">{{ picked.food_unit }}</span>
            <span v-if="qty && Number(qty) > 0" class="text-sm ml-auto">
              ≈ {{ Math.round(Number(qty) * picked.calories_per_unit) }} kcal
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
              No recent foods — search the library to add one.
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
              No matches. Add this food in the
              <RouterLink to="/library" class="underline">Food Library</RouterLink>
              first.
            </div>
            <button
              v-for="food in results"
              :key="food.id"
              type="button"
              class="text-left p-3 rounded-lg border border-border hover:bg-muted transition-colors flex items-center justify-between gap-3"
              @click="pickLibrary(food)"
            >
              <div class="min-w-0">
                <div class="font-medium truncate">{{ food.name }}</div>
                <div class="text-xs text-muted-foreground">
                  {{ food.calories_per_unit }} kcal / 1 {{ food.unit }}
                </div>
              </div>
              <Badge variant="outline">{{ food.unit }}</Badge>
            </button>
          </div>
        </template>
      </div>
    </div>
  </Teleport>
</template>
