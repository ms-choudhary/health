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
  CreateUserPayload,
  UpdateUserPayload,
  Recipe,
  RecipeListItem,
  RecipeWithIngredients,
  RecipePayload,
  LogRecipePayload,
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
  createUser: (payload: CreateUserPayload) =>
    request<User>(`${BASE}/users`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateUser: (id: number, payload: UpdateUserPayload) =>
    request<User>(`${BASE}/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
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

  logRecipe: (userId: number, payload: LogRecipePayload) =>
    request<LogEntry[]>(`${BASE}/users/${userId}/log/recipe`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  deleteLogRecipeGroup: (userId: number, date: string, sourceRecipeId: number) =>
    request<void>(
      `${BASE}/users/${userId}/log/recipe?date=${date}&source_recipe_id=${sourceRecipeId}`,
      { method: 'DELETE' },
    ),

  listRecipes: (search = '') =>
    request<RecipeListItem[]>(`${BASE}/recipes?q=${encodeURIComponent(search)}`),
  getRecipe: (id: number) =>
    request<RecipeWithIngredients>(`${BASE}/recipes/${id}`),
  createRecipe: (payload: RecipePayload) =>
    request<Recipe>(`${BASE}/recipes`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  updateRecipe: (id: number, payload: RecipePayload) =>
    request<Recipe>(`${BASE}/recipes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  deleteRecipe: (id: number) =>
    request<void>(`${BASE}/recipes/${id}`, { method: 'DELETE' }),

  metricsRange: (userId: number, from: string, to: string) =>
    request<DailyMetric[]>(`${BASE}/users/${userId}/metrics?from=${from}&to=${to}`),
  saveMetrics: (userId: number, payload: MetricsUpdate) =>
    request<DailyMetric>(`${BASE}/users/${userId}/metrics`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),

  calorieHint: (name: string) =>
    request<{ hint: string }>(`${BASE}/ai/calorie-hint`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
}
