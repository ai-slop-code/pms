// Helpers + label metadata for the closure / external-sale labels added in
// PMS_14. Keep this file UI-agnostic so it can be reused across the calendar
// and list views.

import type { Occupancy } from '@/api/types/occupancy'
import type { OccupancyBadgeTone } from './status'

export type ClosureState = 'closed' | 'external_sale'

export function closureLabel(state?: ClosureState | null | string): string {
  switch (state) {
    case 'closed':
      return 'Closed'
    case 'external_sale':
      return 'Externally sold'
    default:
      return ''
  }
}

export function closureTone(state?: ClosureState | null | string): OccupancyBadgeTone {
  switch (state) {
    case 'closed':
      return 'neutral'
    case 'external_sale':
      return 'info'
    default:
      return 'neutral'
  }
}

export function isLabelled(o: Pick<Occupancy, 'closure_state'>): boolean {
  return o.closure_state === 'closed' || o.closure_state === 'external_sale'
}

export function formatExternalAmount(o: Occupancy): string {
  if (o.closure_state !== 'external_sale' || o.external_net_amount_cents == null) return ''
  const amount = (o.external_net_amount_cents / 100).toFixed(2)
  const ccy = o.external_currency || ''
  return ccy ? `${amount} ${ccy}` : amount
}

export const closureCategories = [
  { value: 'owner_stay', label: 'Owner stay' },
  { value: 'maintenance', label: 'Maintenance' },
  { value: 'soft_block', label: 'Soft block' },
  { value: 'other', label: 'Other' },
] as const

export const externalChannels = [
  { value: 'airbnb', label: 'Airbnb' },
  { value: 'direct', label: 'Direct booking' },
  { value: 'walk_in', label: 'Walk-in' },
  { value: 'other', label: 'Other' },
] as const
