// Booking-payouts module API types — /api/properties/:id/finance/booking-payouts/*.

export interface BookingPayoutRow {
  id: number
  reference_number: string
  payout_id?: string
  row_type?: string
  check_in_date?: string
  check_out_date?: string
  guest_name?: string
  reservation_status?: string
  currency?: string
  payment_status?: string
  amount_cents?: number
  commission_cents?: number
  payment_service_fee_cents?: number
  net_cents: number
  payout_date: string
  transaction_id?: number
  occupancy_id?: number
  occupancy_start_at?: string
  occupancy_end_at?: string
  occupancy_summary?: string
  linked_invoice_id?: number | null
}

/**
 * Occupancy candidate shown in the BookingPayoutsView "link stay" picker.
 * Different shape from {@link ./invoice.InvoiceOccupancyOption}.
 */
export interface BookingPayoutOccupancyOption {
  id: number
  source_event_uid: string
  start_at: string
  end_at: string
  raw_summary: string
  status: string
}
