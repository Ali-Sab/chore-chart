export interface ChoreEntry {
  name: string
  person: string
}

export type Schedule = Record<string, ChoreEntry[]>

export type Filter = string // 'all' or a person's name

export interface GroupInfo {
  id: number
  name: string
  members: string[]
}

export interface ChoreInfo {
  id: number
  name: string
  assigned_to: string
  cadence_weeks: number
}
