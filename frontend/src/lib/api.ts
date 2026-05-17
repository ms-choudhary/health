import type {
  User,
  Food,
  LogEntry,
  RecentFood,
  DailyMetric,
  MetricsUpdate,
  TodaySummary,
  AddLogPayload,
  CreateFoodPayload,
} from './types'

const BASE = '/api'

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  })
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) msg = body.error
    } catch {
      // ignore body parse failure
    }
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  listUsers: () => request<User[]>(`${BASE}/users`),
  createUser: (name: string) =>
    request<User>(`${BASE}/users`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteUser: (id: number) =>
    request<void>(`${BASE}/users/${id}`, { method: 'DELETE' }),

  todaySummary: (userId: number) =>
    request<TodaySummary>(`${BASE}/users/${userId}/today`),

  listFoods: (search = '') =>
    request<Food[]>(`${BASE}/foods?q=${encodeURIComponent(search)}`),
  createFood: (payload: CreateFoodPayload) =>
    request<Food>(`${BASE}/foods`, { method: 'POST', body: JSON.stringify(payload) }),
  deleteFood: (id: number) =>
    request<void>(`${BASE}/foods/${id}`, { method: 'DELETE' }),

  getLog: (userId: number, date: string) =>
    request<LogEntry[]>(`${BASE}/users/${userId}/log?date=${date}`),
  addLog: (userId: number, payload: AddLogPayload) =>
    request<LogEntry>(`${BASE}/users/${userId}/log`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deleteLog: (userId: number, entryId: number) =>
    request<void>(`${BASE}/users/${userId}/log/${entryId}`, { method: 'DELETE' }),
  recentFoods: (userId: number) =>
    request<RecentFood[]>(`${BASE}/users/${userId}/recent-foods`),

  metricsRange: (userId: number, from: string, to: string) =>
    request<DailyMetric[]>(`${BASE}/users/${userId}/metrics?from=${from}&to=${to}`),
  saveMetrics: (userId: number, payload: MetricsUpdate) =>
    request<DailyMetric>(`${BASE}/users/${userId}/metrics`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
}
