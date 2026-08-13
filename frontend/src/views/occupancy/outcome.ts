import type { StayOutcome } from '@/api/types/occupancy'
import type { OccupancyBadgeTone } from './status'

export function stayOutcomeLabel(outcome?: StayOutcome | null | string): string {
  switch (outcome) {
    case 'cancelled_non_refundable':
      return 'Cancelled: non-refundable'
    case 'no_show':
      return 'No-show'
    default:
      return ''
  }
}

export function stayOutcomeTone(outcome?: StayOutcome | null | string): OccupancyBadgeTone {
  switch (outcome) {
    case 'cancelled_non_refundable':
      return 'warning'
    case 'no_show':
      return 'info'
    default:
      return 'neutral'
  }
}
