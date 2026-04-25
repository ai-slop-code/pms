import { formatEuros } from '@/utils/format'
import type {
  InvoiceOccupancyOption as OccupancyOption,
  InvoiceBookingPayoutOption as BookingPayoutOption,
} from '@/api/types/invoice'

export const eur = (cents?: number | null): string => formatEuros(cents ?? 0)

export function today(): string {
  return new Date().toISOString().slice(0, 10)
}

export function fmt(ts?: string): string {
  if (!ts) return '—'
  return new Date(ts).toLocaleString()
}

export function fmtDay(ts?: string): string {
  if (!ts) return '—'
  return new Date(ts).toLocaleDateString()
}

export function occupancyOptionLabel(o: OccupancyOption): string {
  const g = o.guest_display_name || o.summary || 'Stay'
  const payout = o.has_payout_data ? ' · payout' : ''
  return `#${o.id} ${g}${payout} (${fmtDay(o.start_at)}–${fmtDay(o.end_at)})`
}

export function payoutBillableCents(p: BookingPayoutOption): number {
  const a = p.amount_cents
  if (a != null && a > 0) return a
  return p.net_cents
}

export function payoutOptionLabel(p: BookingPayoutOption): string {
  const summary = (p.payout_summary || p.host_name || p.guest_name || p.occupancy_summary || '').trim()
  const summaryBit = summary ? `${summary} · ` : ''
  const inv = p.linked_invoice_id ? ` · invoiced #${p.linked_invoice_id}` : ''
  return `${summaryBit}${p.reference_number} · ${eur(payoutBillableCents(p))}${inv}`
}
