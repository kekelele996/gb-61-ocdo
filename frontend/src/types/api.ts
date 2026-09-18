export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface PageData<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

export interface UserInfo {
  id: number
  username: string
  email: string
  nickname: string
  avatar: string
  bio: string
  role: 'user' | 'admin'
  created_at: string
}

export interface CareReminder {
  id: number
  user_id: number
  plant_species_id: number
  task_title: string
  remind_date: string
  frequency: string
  frequency_text?: string
  status: 'pending' | 'done' | 'overdue'
  created_at: string
}

export interface UserGarden {
  id: number
  user_id: number
  plant_species_id: number
  nickname: string
  owned_since: string
  location: string
  care_reminder_id: number
  created_at: string
}

// GardenItem is the enriched garden entry returned by GET /gardens and embedded
// in the POST /gardens result.
export interface GardenItem extends UserGarden {
  plant_name: string
  watering_frequency_text: string
  watering_plan_text: string
  first_watering_date: string | null
}

// GardenAddResult is the POST /gardens response. duplicated=true means the
// submission was deduped (refresh/concurrent repeat): the existing single entry
// is returned without creating another reminder.
export interface GardenAddResult extends GardenItem {
  duplicated: boolean
}

export interface DiseasePest {
  id: number
  plant_species_id: number
  name: string
  symptoms: string
  cause: string
  treatment: string
  recommended_medicine: string
  images: string
  keywords: string
  created_at: string
}

export interface Question {
  id: number
  user_id: number
  title: string
  content: string
  images: string
  status: string
  created_at: string
}

export interface Answer {
  id: number
  question_id: number
  user_id: number
  content: string
  is_best: boolean
  like_count: number
  created_at: string
}
