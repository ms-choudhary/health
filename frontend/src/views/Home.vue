<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { api } from '@/lib/api'
import type { TodaySummary } from '@/lib/types'
import { formatNumber } from '@/lib/utils'
import Card from '@/components/ui/Card.vue'
import Button from '@/components/ui/Button.vue'
import Input from '@/components/ui/Input.vue'
import Dialog from '@/components/ui/Dialog.vue'
import Avatar from '@/components/ui/Avatar.vue'
import DonutChart from '@/components/DonutChart.vue'
import { BookOpen, Plus, ChevronRight } from 'lucide-vue-next'

const router = useRouter()
const userStore = useUserStore()
const summaries = ref<Record<number, TodaySummary>>({})
const loading = ref(true)
const showAdd = ref(false)
const newName = ref<string>('')
const newTarget = ref<string>('2000')
const adding = ref(false)
const errMsg = ref('')

async function loadSummaries() {
  const entries = await Promise.all(
    userStore.users.map(async (u) => {
      const s = await api.todaySummary(u.id)
      return [u.id, s] as const
    }),
  )
  summaries.value = Object.fromEntries(entries)
}

async function load() {
  loading.value = true
  try {
    await userStore.load()
    await loadSummaries()
  } finally {
    loading.value = false
  }
}

async function submitAdd() {
  const name = newName.value.trim()
  const t = Number(newTarget.value)
  if (!name) return
  if (!Number.isFinite(t) || t <= 0) {
    errMsg.value = 'Daily calorie target must be a positive number'
    return
  }
  adding.value = true
  errMsg.value = ''
  try {
    const u = await userStore.add({ name, target_calories: Math.round(t) })
    summaries.value = { ...summaries.value, [u.id]: { consumed: 0, target: u.target_calories } }
    newName.value = ''
    newTarget.value = '2000'
    showAdd.value = false
  } catch (e) {
    errMsg.value = e instanceof Error ? e.message : 'Failed to add user'
  } finally {
    adding.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="max-w-lg mx-auto p-4 sm:p-6 flex flex-col gap-4">
    <header class="flex items-center justify-between">
      <h1 class="text-2xl font-bold tracking-tight">Health Tracker</h1>
      <Button variant="outline" size="sm" @click="router.push('/library')">
        <BookOpen class="h-4 w-4" />
        Food Library
      </Button>
    </header>

    <div v-if="loading" class="flex flex-col gap-3">
      <div
        v-for="i in 2"
        :key="i"
        class="h-[72px] rounded-lg bg-muted animate-pulse"
      />
    </div>

    <div v-else-if="userStore.users.length === 0" class="text-center py-12 text-muted-foreground">
      No users yet — tap "Add user" to start.
    </div>

    <div v-else class="flex flex-col gap-3">
      <Card
        v-for="u in userStore.users"
        :key="u.id"
        class="cursor-pointer transition-colors hover:bg-muted"
      >
        <div class="p-4 flex items-center gap-4" @click="router.push(`/user/${u.id}`)">
          <Avatar :initials="u.avatar" :seed="u.id" :size="44" />
          <div class="flex-1 min-w-0">
            <div class="font-semibold">{{ u.name }}</div>
            <div class="text-xs text-muted-foreground mt-0.5">
              <template v-if="summaries[u.id]?.target">
                {{ formatNumber(summaries[u.id].consumed) }} /
                {{ formatNumber(summaries[u.id].target) }} kcal today
              </template>
              <template v-else>
                {{ formatNumber(summaries[u.id]?.consumed ?? 0) }} kcal today · no target set
              </template>
            </div>
          </div>
          <DonutChart
            :consumed="summaries[u.id]?.consumed ?? 0"
            :target="summaries[u.id]?.target ?? 0"
            :size="52"
          />
          <ChevronRight class="h-4 w-4 text-muted-foreground" />
        </div>
      </Card>
    </div>

    <Button variant="outline" @click="showAdd = true">
      <Plus class="h-4 w-4" />
      Add user
    </Button>
  </div>

  <Dialog v-model:open="showAdd" title="New user">
    <div class="flex flex-col gap-3">
      <Input v-model="newName" placeholder="Name" @keyup.enter="submitAdd" />
      <div class="flex flex-col gap-1">
        <Input
          v-model="newTarget"
          type="number"
          inputmode="numeric"
          min="1"
          step="50"
          placeholder="Daily calorie target"
          @keyup.enter="submitAdd"
        />
        <p class="text-xs text-muted-foreground">Daily calorie target (kcal)</p>
      </div>
      <p v-if="errMsg" class="text-sm text-destructive">{{ errMsg }}</p>
      <div class="flex justify-end gap-2">
        <Button variant="ghost" @click="showAdd = false">Cancel</Button>
        <Button :disabled="adding || !newName.trim()" @click="submitAdd">
          {{ adding ? 'Adding…' : 'Create' }}
        </Button>
      </div>
    </div>
  </Dialog>
</template>
