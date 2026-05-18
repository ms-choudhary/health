export interface User {
  id: number
  name: string
  avatar: string
  target_calories: number
  created_at: string
}

export interface Food {
  id: number
  name: string
  unit: string
  calories_per_unit: number
  created_at: string
}

export interface LogEntry {
  id: number
  user_id: number
  food_id: number | null
  date: string
  food_name: string
  food_unit: string
  calories_per_unit: number
  quantity: number
  calories: number
  source_recipe_id: number | null
  source_recipe_name: string | null
}

export interface RecentFood {
  food_name: string
  food_unit: string
  calories_per_unit: number
  food_id: number | null
  last_quantity: number
}

export interface DailyMetric {
  id: number
  user_id: number
  date: string
  weight: number | null
  steps: number | null
  calories_consumed: number
}

export interface MetricsUpdate {
  date: string
  weight: number | null
  steps: number | null
}

export interface CreateUserPayload {
  name: string
  target_calories: number
}

export interface UpdateUserPayload {
  name?: string
  target_calories?: number
}

export interface TodaySummary {
  consumed: number
  target: number
}

export interface AddLogPayload {
  food_id: number | null
  food_name: string
  food_unit: string
  calories_per_unit: number
  quantity: number
  date: string
}

export interface CreateFoodPayload {
  name: string
  unit: string
  calories_per_unit: number
}

export interface UpdateFoodPayload {
  calories_per_unit: number
}

export interface Recipe {
  id: number
  name: string
  created_at: string
}

export interface RecipeListItem extends Recipe {
  total_calories: number
}

export interface RecipeIngredient {
  id: number
  recipe_id: number
  food_id: number
  quantity: number
  food_name: string
  food_unit: string
  calories_per_unit: number
}

export interface RecipeWithIngredients extends RecipeListItem {
  ingredients: RecipeIngredient[]
}

export interface RecipeIngredientInput {
  food_id: number
  quantity: number
}

export interface RecipePayload {
  name: string
  ingredients: RecipeIngredientInput[]
}

export interface LogRecipePayload {
  recipe_id: number
  scale: number
  date: string
}

export type Pickable =
  | { kind: 'food'; food: Food }
  | { kind: 'recipe'; recipe: RecipeListItem }
