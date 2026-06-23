export interface User {
  id: number
  name: string
  avatar: string
  target_calories: number
  target_protein: number
  created_at: string
}

export interface Ingredient {
  id: number
  name: string
  unit: string
  calories_per_unit: number
  protein_per_unit: number
  created_at: string
}

export interface LogEntry {
  id: number
  user_id: number
  ingredient_id: number | null
  date: string
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: number
  calories: number
  protein: number
  recipe_group_id: number | null
  source_recipe_name: string | null
  source_recipe_servings: number | null
}

export interface DailyMetric {
  id: number
  user_id: number
  date: string
  weight: number | null
  steps: number | null
  calories_consumed: number
  protein_consumed: number
}

export interface MetricsUpdate {
  date: string
  weight: number | null
  steps: number | null
}

export interface CreateUserPayload {
  name: string
  target_calories: number
  target_protein: number
}

export interface UpdateUserPayload {
  name?: string
  target_calories?: number
  target_protein?: number
}

export interface TodaySummary {
  consumed: number
  target: number
  protein_consumed: number
  target_protein: number
}

export interface CreateIngredientPayload {
  name: string
  unit: string
  calories_per_unit: number
  protein_per_unit: number
}

export interface UpdateIngredientPayload {
  calories_per_unit: number
  protein_per_unit: number
}

export interface FoodTag {
  id: number
  name: string
  created_at: string
}

export interface FoodTagWithCount extends FoodTag {
  recipe_count: number
}

export interface Recipe {
  id: number
  name: string
  created_at: string
}

export interface RecipeListItem extends Recipe {
  total_calories: number
  total_protein: number
  food_tags: FoodTag[]
}

// Minimal recipe reference (id + name), e.g. recipes that use a given ingredient.
export interface RecipeRef {
  id: number
  name: string
}

export interface RecipeIngredient {
  id: number
  recipe_id: number
  ingredient_id: number
  quantity: number
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
}

export interface RecipeWithIngredients extends RecipeListItem {
  ingredients: RecipeIngredient[]
}

export interface RecipeIngredientInput {
  ingredient_id: number
  quantity: number
}

export interface RecipePayload {
  name: string
  ingredients: RecipeIngredientInput[]
  food_tag_ids: number[]
}

export interface LogRecipePayload {
  recipe_id: number
  servings: number
  date: string
}

export interface CustomRecipeItem {
  ingredient_id: number | null
  ingredient_name: string
  ingredient_unit: string
  calories_per_unit: number
  protein_per_unit: number
  quantity: number
}

export interface LogCustomRecipePayload {
  name: string
  date: string
  items: CustomRecipeItem[]
}

export interface Exercise {
  id: number
  name: string
  notes: string
  created_at: string
}

export interface ExerciseTag {
  id: number
  name: string
  created_at: string
}

export interface ExerciseTagWithCount extends ExerciseTag {
  exercise_count: number
}

export interface ExerciseWithTags extends Exercise {
  tags: ExerciseTag[]
}

export interface ExerciseSet {
  id: number
  user_id: number
  exercise_id: number | null
  exercise_name: string
  date: string
  weight: number
  reps: number
  unit: string
}

export interface SetInput {
  weight: number
  reps: number
}

export interface AddSetsPayload {
  exercise_id: number
  date: string
  unit?: string
  sets: SetInput[]
}

export interface ExercisePayload {
  name: string
  notes: string
  exercise_tag_ids: number[]
}

export interface ProgressPoint {
  date: string
  total_volume: number
  breakdown: string
}

export interface ExerciseProgress {
  exercise_id: number | null
  exercise_name: string
  points: ProgressPoint[]
}
