export const DOW_MON = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']
export const DOW_SUN = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
export const monthLabels = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
]

import { formatEuros } from '@/utils/format'

export interface UnsoldRange {
  from: string
  to: string
  nights: number
  prev_guest?: string
  next_guest?: string
}

export const eur = (cents?: number | null): string => formatEuros(cents ?? 0)

export function pct(v: number): string {
  return `${(v * 100).toFixed(1)}%`
}

export function freshnessTone(level?: string): 'success' | 'warning' | 'danger' {
  if (level === 'ok') return 'success'
  if (level === 'warn') return 'warning'
  return 'danger'
}

export function freshnessLabel(level?: string): string {
  if (level === 'ok') return 'Fresh'
  if (level === 'warn') return 'Warning'
  return 'Stale'
}

export function dowIndex(dow: number, weekStartsOn: 'monday' | 'sunday'): number {
  return weekStartsOn === 'monday' ? (dow === 0 ? 6 : dow - 1) : dow
}

export function dowLabel(dow: number, weekStartsOn: 'monday' | 'sunday'): string {
  const arr = weekStartsOn === 'monday' ? DOW_MON : DOW_SUN
  return arr[dowIndex(dow, weekStartsOn)] ?? ''
}

export function weekdayOfIso(iso: string, weekStartsOn: 'monday' | 'sunday'): string {
  if (!iso) return ''
  const d = new Date(`${iso}T00:00:00`)
  if (isNaN(d.getTime())) return ''
  return dowLabel(d.getDay(), weekStartsOn)
}

export function leadBucketLabel(b: string): string {
  switch (b) {
    case '0-3': return '0–3 days'
    case '4-14': return '4–14 days'
    case '15-45': return '15–45 days'
    case '46-90': return '46–90 days'
    case '91+': return '91+ days'
    default: return b
  }
}

export function losBucketLabel(b: string): string {
  switch (b) {
    case '1': return '1 night'
    case '2': return '2 nights'
    case '3': return '3 nights'
    case '4-5': return '4–5 nights'
    case '6-7': return '6–7 nights'
    case '8-14': return '8–14 nights'
    case '15+': return '15+ nights'
    default: return b
  }
}

export function cancellationBucketLabel(b: string): string {
  switch (b) {
    case '0-3': return '0–3 days'
    case '4-14': return '4–14 days'
    case '15-45': return '15–45 days'
    case '46+': return '46+ days'
    default: return b
  }
}

export function heatCellColor(rate: number): string {
  const clamped = Math.max(0, Math.min(1, rate))
  const light = 243 - Math.round(clamped * 95)
  const g = 244 - Math.round(clamped * 150)
  const b = 246 - Math.round(clamped * 176)
  return `rgb(${light}, ${g}, ${b})`
}

export function todayIso(): string {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${dd}`
}

export function addDaysIso(iso: string, days: number): string {
  const d = new Date(`${iso}T00:00:00`)
  d.setDate(d.getDate() + days)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${dd}`
}
