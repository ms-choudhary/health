export interface User {
  id: number
  name: string
  avatar: string
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
  target_calories: number | null
  calories_consumed: number
}

export interface MetricsUpdate {
  date: string
  weight: number | null
  steps: number | null
  target_calories: number | null
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
