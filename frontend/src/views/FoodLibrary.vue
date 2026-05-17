<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import type { Food } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Badge from '@/components/ui/Badge.vue'
import Dialog from '@/components/ui/Dialog.vue'
import { ChevronLeft, Search, Trash2, Plus } from 'lucide-vue-next'

const UNITS = ['g', 'ml', 'oz', 'piece', 'tbsp', 'cup', 'serving']

const router = useRouter()
const query = ref('')
const foods = ref<Food[]>([])
const loading = ref(true)
const showAdd = ref(false)
const newFood = ref<{ name: string; unit: string; calories: string }>({
  name: '',
  unit: 'g',
  calories: '',
})
const saving = ref(false)
const errMsg = ref('')

let searchTimer: number | undefined

async function load() {
  loading.value = true
  try {
    foods.value = await api.listFoods(query.value)
  } finally {
    loading.value = false
  }
}

watch(query, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(load, 200)
})

async function addFood() {
  const name = newFood.value.name.trim()
  const cal = Number(newFood.value.calories)
  if (!name || !Number.isFinite(cal) || cal < 0) {
    errMsg.value = 'Enter a name and non-negative calorie value'
    return
  }
  saving.value = true
  errMsg.value = ''
  try {
    await api.createFood({ name, unit: newFood.value.unit, calories_per_unit: cal })
    showAdd.value = false
    newFood.value = { name: '', unit: 'g', calories: '' }
    await load()
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to add food'
  } finally {
    saving.value = false
  }
}

async function deleteFood(id: number) {
  if (!confirm('Delete this food from the library?')) return
  await api.deleteFood(id)
  await load()
}

onMounted(load)
</script>

<template>
  <div class="max-w-lg mx-auto p-4 sm:p-6 flex flex-col gap-4">
    <header class="flex items-center gap-2">
      <Button variant="ghost" size="icon" @click="router.push('/')">
        <ChevronLeft class="h-5 w-5" />
      </Button>
      <h1 class="text-xl font-bold">Food Library</h1>
    </header>

    <div class="relative">
      <Input v-model="query" type="search" placeholder="Search food…" />
      <Search class="absolute right-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
    </div>

    <div v-if="loading" class="flex flex-col gap-2">
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
          <Button variant="ghost" size="icon" @click="deleteFood(f.id)">
            <Trash2 class="h-4 w-4" />
          </Button>
        </div>
      </Card>
    </div>

    <Button class="mt-2" @click="showAdd = true">
      <Plus class="h-4 w-4" />
      Add food
    </Button>
  </div>

  <Dialog v-model:open="showAdd" title="New food">
    <div class="flex flex-col gap-3">
      <Input v-model="newFood.name" placeholder="Food name" />
      <div class="flex gap-2">
        <Input
          v-model="newFood.calories"
          type="number"
          inputmode="decimal"
          placeholder="Calories"
          min="0"
          step="0.1"
        />
        <select
          v-model="newFood.unit"
          class="h-9 rounded-md border border-input bg-background px-2 text-sm"
        >
          <option v-for="u in UNITS" :key="u" :value="u">{{ u }}</option>
        </select>
      </div>
      <p class="text-xs text-muted-foreground">Calories per 1 {{ newFood.unit }}</p>
      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="showAdd = false">Cancel</Button>
        <Button :disabled="saving" @click="addFood">{{ saving ? 'Saving…' : 'Add' }}</Button>
      </div>
    </div>
  </Dialog>
</template>
