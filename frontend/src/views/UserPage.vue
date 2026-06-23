<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import Avatar from '@/components/ui/Avatar.vue'
import Button from '@/components/ui/Button.vue'
import TodayTab from '@/components/TodayTab.vue'
import ProgressTab from '@/components/ProgressTab.vue'
import LogTab from '@/components/LogTab.vue'
import { ChevronLeft } from 'lucide-vue-next'

type Tab = 'today' | 'progress' | 'log'

const props = defineProps<{ userId: number }>()
const router = useRouter()
const userStore = useUserStore()

const tab = ref<Tab>('today')
const user = computed(() => userStore.findById(props.userId))

const tabs: { id: Tab; label: string }[] = [
  { id: 'today', label: 'Today' },
  { id: 'progress', label: 'Progress' },
  { id: 'log', label: 'Log' },
]

const activeComponent = computed(() => {
  if (tab.value === 'today') return TodayTab
  if (tab.value === 'progress') return ProgressTab
  return LogTab
})

onMounted(async () => {
  if (userStore.users.length === 0) await userStore.load()
})
</script>

<template>
  <div class="max-w-lg mx-auto flex flex-col h-full">
    <header class="flex items-center gap-3 p-4">
      <Button variant="ghost" size="icon" @click="router.push('/')">
        <ChevronLeft class="h-5 w-5" />
      </Button>
      <Avatar v-if="user" :initials="user.avatar" :seed="user.id" :size="36" />
      <h1 class="text-xl font-bold">{{ user?.name ?? 'User' }}</h1>
    </header>

    <nav class="flex gap-1 mx-4 mb-4 p-1 rounded-xl bg-card border border-border">
      <button
        v-for="t in tabs"
        :key="t.id"
        type="button"
        class="flex-1 h-9 rounded-lg text-sm transition-colors"
        :class="tab === t.id
          ? 'bg-primary text-primary-foreground font-bold'
          : 'text-muted-foreground font-medium hover:bg-secondary'"
        @click="tab = t.id"
      >
        {{ t.label }}
      </button>
    </nav>

    <main class="flex-1 overflow-y-auto px-4 pb-12">
      <component :is="activeComponent" :user-id="userId" />
    </main>
  </div>
</template>
