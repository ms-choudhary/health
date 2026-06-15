<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { format, parseISO } from 'date-fns'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/utils'
import type { LogEntry } from '@/lib/types'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import AddRecipeDrawer from '@/components/AddRecipeDrawer.vue'
import { Plus, Trash2 } from 'lucide-vue-next'

interface RecipeGroup {
  recipeId: number
  recipeName: string
  servings: number | null
  entries: LogEntry[]
  totalCalories: number
  totalProtein: number
}
interface DayLog {
  date: string
  displayDate: string
  groups: RecipeGroup[]
  totalCalories: number
  totalProtein: number
}

const props = defineProps<{ userId: number }>()

const today = format(new Date(), 'yyyy-MM-dd')
const entries = ref<LogEntry[]>([])
const loadingLog = ref(false)
const showDrawer = ref(false)

const days = computed<DayLog[]>(() => {
  const byDate = new Map<string, DayLog>()
  const order: string[] = []
  for (const e of entries.value) {
    if (e.source_recipe_id == null) continue
    let day = byDate.get(e.date)
    if (!day) {
      day = {
        date: e.date,
        displayDate: format(parseISO(e.date), 'MMMM d, yyyy'),
        groups: [],
        totalCalories: 0,
        totalProtein: 0,
      }
      byDate.set(e.date, day)
      order.push(e.date)
    }
    let g = day.groups.find((x) => x.recipeId === e.source_recipe_id)
    if (!g) {
      g = {
        recipeId: e.source_recipe_id,
        recipeName: e.source_recipe_name ?? 'Recipe',
        servings: e.source_recipe_servings,
        entries: [],
        totalCalories: 0,
        totalProtein: 0,
      }
      day.groups.push(g)
    }
    g.entries.push(e)
    g.totalCalories += e.calories
    g.totalProtein += e.protein
    day.totalCalories += e.calories
    day.totalProtein += e.protein
  }
  return order.map((d) => byDate.get(d)!)
})

async function loadLog(): Promise<void> {
  loadingLog.value = true
  try {
    entries.value = await api.getLog(props.userId)
  } finally {
    loadingLog.value = false
  }
}

async function removeRecipeGroup(date: string, recipeId: number, name: string): Promise<void> {
  if (!confirm(`Remove “${name}” from this day's log?`)) return
  await api.deleteLogRecipeGroup(props.userId, date, recipeId)
  entries.value = entries.value.filter(
    (e) => !(e.date === date && e.source_recipe_id === recipeId),
  )
}

function onAdded(): void {
  showDrawer.value = false
  void loadLog()
}

onMounted(loadLog)
</script>

<template>
  <div class="flex flex-col gap-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-bold">Logs</h2>
      <Button size="sm" @click="showDrawer = true">
        <Plus class="h-4 w-4" />
        Create
      </Button>
    </div>

    <div v-if="loadingLog" class="flex flex-col gap-3">
      <div v-for="i in 3" :key="i" class="h-32 rounded-xl bg-card animate-pulse" />
    </div>
    <div v-else-if="days.length === 0" class="text-center py-12 text-muted-foreground text-sm italic">
      Nothing logged yet — tap “Create” to add a recipe.
    </div>

    <template v-else>
      <div v-for="day in days" :key="day.date" class="flex flex-col gap-2">
        <div class="font-mono text-lg font-semibold uppercase tracking-wide text-foreground">
          {{ day.displayDate }}
        </div>
        <Card class="overflow-hidden">
          <table class="w-full text-sm">
            <thead class="text-xs text-muted-foreground">
              <tr class="border-b border-border">
                <th class="text-left font-medium px-3 py-2">Item</th>
                <th class="text-right font-medium px-3 py-2 w-16">Qty</th>
                <th class="text-right font-medium px-3 py-2 w-16">Cal</th>
                <th class="text-right font-medium px-3 py-2 w-16">Prot</th>
                <th class="w-9" />
              </tr>
            </thead>
            <tbody>
              <template v-for="g in day.groups" :key="`r-${day.date}-${g.recipeId}`">
                <tr class="border-b border-border bg-muted/40">
                  <td class="px-3 py-2">
                    <div class="flex items-center gap-2">
                      <span class="font-medium">{{ g.recipeName }}</span>
                      <span class="text-[10px] uppercase tracking-wide text-muted-foreground rounded bg-muted px-1.5 py-0.5">
                        Recipe
                      </span>
                    </div>
                    <div v-if="g.servings != null" class="text-xs text-muted-foreground font-mono">
                      {{ formatNumber(g.servings, g.servings % 1 ? 1 : 0) }}
                      serving{{ g.servings === 1 ? '' : 's' }}
                    </div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground">—</td>
                  <td class="text-right px-3 py-2 font-medium font-mono">{{ Math.round(g.totalCalories) }}</td>
                  <td class="text-right px-3 py-2 font-medium font-mono">{{ formatNumber(g.totalProtein, 1) }}</td>
                  <td class="px-1">
                    <Button variant="ghost" size="icon" @click="removeRecipeGroup(day.date, g.recipeId, g.recipeName)">
                      <Trash2 class="h-4 w-4" />
                    </Button>
                  </td>
                </tr>
                <tr v-for="e in g.entries" :key="`re-${e.id}`" class="border-b border-border last:border-0">
                  <td class="px-3 py-2 pl-6">
                    <div class="text-muted-foreground">↳ {{ e.ingredient_name }}</div>
                    <div class="text-xs text-muted-foreground">{{ e.ingredient_unit }}</div>
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">
                    {{ formatNumber(e.quantity, e.quantity % 1 ? 1 : 0) }}
                  </td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">{{ Math.round(e.calories) }}</td>
                  <td class="text-right px-3 py-2 text-muted-foreground font-mono">{{ formatNumber(e.protein, 1) }}</td>
                  <td class="px-1" />
                </tr>
              </template>
            </tbody>
          </table>
          <div class="px-3 py-3 border-t border-border flex justify-between font-semibold font-mono">
            <span>Total</span>
            <span>
              <span class="text-primary">{{ formatNumber(Math.round(day.totalCalories)) }} kcal</span>
              <span class="ml-3 text-muted-foreground">{{ formatNumber(day.totalProtein, 1) }} g</span>
            </span>
          </div>
        </Card>
      </div>
    </template>
  </div>

  <AddRecipeDrawer
    v-if="showDrawer"
    :user-id="userId"
    :date="today"
    @close="showDrawer = false"
    @added="onAdded"
  />
</template>
