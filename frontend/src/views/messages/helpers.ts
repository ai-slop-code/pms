import type { MessagesStay as Stay } from '@/api/types/messages'

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

export function occLabel(occ: Stay): string {
  const name = occ.display_name || `#${occ.id}`
  const start = fmtDate(occ.check_in_date)
  const end = fmtDate(occ.check_out_date)
  return `${name} (${start} → ${end})`
}
