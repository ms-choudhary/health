import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { User } from '@/lib/types'
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

  async function add(name: string): Promise<User> {
    const u = await api.createUser(name)
    users.value = [...users.value, u]
    return u
  }

  function findById(id: number): User | undefined {
    return users.value.find((u) => u.id === id)
  }

  return { users, loading, load, add, findById }
})
