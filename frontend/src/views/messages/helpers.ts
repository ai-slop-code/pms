import type { MessagesOccupancy as Occupancy } from '@/api/types/messages'

export const TEMPLATE_TYPE_LABELS: Record<string, string> = {
  check_in: 'Check-in',
  cleaning_staff: 'Cleaning staff',
}

export const LANG_LABELS: Record<string, string> = {
  en: 'English',
  sk: 'Slovenčina',
  de: 'Deutsch',
  uk: 'Українська',
  hu: 'Magyar',
}

export function fmtDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString('en-GB', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
    })
  } catch {
    return iso
  }
}

export function fmtDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('en-GB', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return iso
  }
}

export function occLabel(occ: Occupancy): string {
  const name = occ.guest_display_name || occ.raw_summary || `#${occ.id}`
  const start = fmtDate(occ.start_at)
  const end = fmtDate(occ.end_at)
  return `${name} (${start} → ${end})`
}
