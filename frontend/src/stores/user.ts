import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { User, CreateUserPayload } from '@/lib/types'
import { api } from '@/lib/api'

export const useUserStore = defineStore('users', () => {
  const users = ref<User[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      users.value = await api.listUsers()
    } finally {
      loading.value = false
    }
  }

  async function add(payload: CreateUserPayload): Promise<User> {
    const u = await api.createUser(payload)
    users.value = [...users.value, u]
    return u
  }

  function upsert(user: User): void {
    const idx = users.value.findIndex((u) => u.id === user.id)
    if (idx >= 0) {
      users.value = [
        ...users.value.slice(0, idx),
        user,
        ...users.value.slice(idx + 1),
      ]
    } else {
      users.value = [...users.value, user]
    }
  }

  function findById(id: number): User | undefined {
    return users.value.find((u) => u.id === id)
  }

  return { users, loading, load, add, upsert, findById }
})
