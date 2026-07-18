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
  named_stay_id?: number
  occupancy_start_at?: string
  occupancy_end_at?: string
  occupancy_summary?: string
  named_stay_display_name?: string
  named_stay_type?: 'booking_com' | 'external' | 'maintenance' | 'personal_use'
  named_stay_check_in_date?: string
  named_stay_check_out_date?: string
  outcome_override?: 'cancelled_non_refundable' | 'no_show' | null
  outcome_override_marked_at?: string | null
  linked_invoice_id?: number | null
  has_payout_data: boolean
  has_statement_data: boolean
}

/**
 * Occupancy candidate shown in the BookingPayoutsView "link stay" picker.
 * Different shape from {@link ./invoice.InvoiceOccupancyOption}.
 */
export interface BookingPayoutStayOption {
  id: number
  display_name: string
  stay_type: 'booking_com' | 'external' | 'maintenance' | 'personal_use'
  check_in_date: string
  check_out_date: string
  status: string
  review_status?: string
  manual_revenue_cents?: number
  has_finance_data: boolean
}

export type BookingPayoutOccupancyOption = BookingPayoutStayOption
