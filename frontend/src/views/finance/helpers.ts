export type FinanceTab = 'overview' | 'transactions' | 'recurring' | 'categories' | 'breakdown'

export const FINANCE_TABS: Array<{ id: string; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'transactions', label: 'Transactions' },
  { id: 'recurring', label: 'Recurring rules' },
  { id: 'categories', label: 'Categories' },
  { id: 'breakdown', label: 'Monthly breakdown' },
]

export const VIZ_PALETTE = [
  'var(--viz-1)',
  'var(--viz-2)',
  'var(--viz-3)',
  'var(--viz-4)',
  'var(--viz-5)',
  'var(--viz-6)',
]

export function displayDirection(value: 'incoming' | 'outgoing' | 'both'): string {
  switch (value) {
    case 'incoming':
      return 'Incoming'
    case 'outgoing':
      return 'Outgoing'
    case 'both':
      return 'Both'
  }
}

export function displaySource(source?: string): string {
  switch (source) {
    case 'manual':
      return 'Manual'
    case 'booking_payout':
      return 'Booking.com payout'
    case 'recurring_rule':
      return 'Recurring rule'
    case 'cleaning_salary':
      return 'Cleaning salary'
    default:
      return source ? source.replaceAll('_', ' ') : '—'
  }
}

export function directionTone(dir: 'incoming' | 'outgoing'): 'success' | 'warning' {
  return dir === 'incoming' ? 'success' : 'warning'
}


export interface TxForm {
  transaction_date: string
  direction: "incoming" | "outgoing"
  amount_eur: number
  category_id: number
  note: string
  attachment: File | null
}

export interface RecurringForm {
  title: string
  category_id: number
  amount_eur: number
  direction: "incoming" | "outgoing"
  start_month: string
  end_month: string
  effective_from: string
}

export interface CategoryForm {
  code: string
  title: string
  direction: "incoming" | "outgoing" | "both"
  counts_toward_property_income: boolean
}
