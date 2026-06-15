import type {
  User,
  Ingredient,
  LogEntry,
  DailyMetric,
  MetricsUpdate,
  TodaySummary,
  CreateIngredientPayload,
  UpdateIngredientPayload,
  CreateUserPayload,
  UpdateUserPayload,
  Recipe,
  RecipeListItem,
  RecipeWithIngredients,
  RecipePayload,
  LogRecipePayload,
  LogCustomRecipePayload,
  FoodTag,
  FoodTagWithCount,
  Exercise,
  ExerciseTag,
  ExerciseTagWithCount,
  ExerciseWithTags,
  ExerciseSet,
  SetInput,
  AddSetsPayload,
  ExercisePayload,
  ExerciseProgress,
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

  listIngredients: (search = '') =>
    request<Ingredient[]>(`${BASE}/ingredients?q=${encodeURIComponent(search)}`),
  createIngredient: (payload: CreateIngredientPayload) =>
    request<Ingredient>(`${BASE}/ingredients`, { method: 'POST', body: JSON.stringify(payload) }),
  updateIngredient: (id: number, payload: UpdateIngredientPayload) =>
    request<Ingredient>(`${BASE}/ingredients/${id}`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    }),
  deleteIngredient: (id: number) =>
    request<void>(`${BASE}/ingredients/${id}`, { method: 'DELETE' }),

  getLog: (userId: number) =>
    request<LogEntry[]>(`${BASE}/users/${userId}/log`),
  deleteLog: (userId: number, entryId: number) =>
    request<void>(`${BASE}/users/${userId}/log/${entryId}`, { method: 'DELETE' }),

  logRecipe: (userId: number, payload: LogRecipePayload) =>
    request<LogEntry[]>(`${BASE}/users/${userId}/log/recipe`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  logCustomRecipe: (userId: number, payload: LogCustomRecipePayload) =>
    request<LogEntry[]>(`${BASE}/users/${userId}/log/custom-recipe`, {
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

  listFoodTags: () => request<FoodTagWithCount[]>(`${BASE}/food-tags`),
  createFoodTag: (name: string) =>
    request<FoodTag>(`${BASE}/food-tags`, {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteFoodTag: (id: number) =>
    request<void>(`${BASE}/food-tags/${id}`, { method: 'DELETE' }),
  recipesByFoodTag: (foodTagId: number) =>
    request<RecipeListItem[]>(`${BASE}/food-tags/${foodTagId}/recipes`),

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

  listExercises: (search = '') =>
    request<ExerciseWithTags[]>(`${BASE}/exercises?q=${encodeURIComponent(search)}`),
  getExercise: (id: number) => request<ExerciseWithTags>(`${BASE}/exercises/${id}`),
  createExercise: (payload: ExercisePayload) =>
    request<Exercise>(`${BASE}/exercises`, { method: 'POST', body: JSON.stringify(payload) }),
  updateExercise: (id: number, payload: ExercisePayload) =>
    request<Exercise>(`${BASE}/exercises/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteExercise: (id: number) =>
    request<void>(`${BASE}/exercises/${id}`, { method: 'DELETE' }),

  listExerciseTags: () => request<ExerciseTagWithCount[]>(`${BASE}/exercise-tags`),
  createExerciseTag: (name: string) =>
    request<ExerciseTag>(`${BASE}/exercise-tags`, { method: 'POST', body: JSON.stringify({ name }) }),
  deleteExerciseTag: (id: number) =>
    request<void>(`${BASE}/exercise-tags/${id}`, { method: 'DELETE' }),
  exercisesByTag: (tagId: number) =>
    request<ExerciseWithTags[]>(`${BASE}/exercise-tags/${tagId}/exercises`),

  getSets: (userId: number) => request<ExerciseSet[]>(`${BASE}/users/${userId}/sets`),
  lastSets: (userId: number, exerciseId: number) =>
    request<SetInput[]>(`${BASE}/users/${userId}/exercises/${exerciseId}/last-sets`),
  addSets: (userId: number, payload: AddSetsPayload) =>
    request<ExerciseSet[]>(`${BASE}/users/${userId}/sets`, { method: 'POST', body: JSON.stringify(payload) }),
  updateSet: (userId: number, setId: number, payload: SetInput) =>
    request<ExerciseSet>(`${BASE}/users/${userId}/sets/${setId}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteSet: (userId: number, setId: number) =>
    request<void>(`${BASE}/users/${userId}/sets/${setId}`, { method: 'DELETE' }),
  exerciseProgress: (userId: number, from: string, to: string) =>
    request<ExerciseProgress[]>(`${BASE}/users/${userId}/exercise-progress?from=${from}&to=${to}`),
}
