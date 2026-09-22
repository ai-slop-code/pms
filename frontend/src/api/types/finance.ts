// Finance module API types — shapes returned by /api/properties/:id/finance/*.

export type FinanceDirection = 'incoming' | 'outgoing'

export interface FinanceCategory {
  id: number
  property_id?: number
  code: string
  title: string
  direction: FinanceDirection | 'both'
  counts_toward_property_income: boolean
}

export interface FinanceTransaction {
  id: number
  transaction_date: string
  direction: FinanceDirection
  amount_cents: number
  category_id?: number
  category_code?: string
  category_title?: string
  note?: string
  source_type: string
  source_reference_id?: string
  is_auto_generated: boolean
  attachment_path?: string
  mapped_to_stay?: boolean
}

export interface FinanceRecurringRule {
  id: number
  title: string
  category_id?: number
  amount_cents: number
  direction: FinanceDirection
  frequency: string
  start_month: string
  end_month?: string
  effective_from: string
  effective_to?: string
  active: boolean
}

export interface FinanceSummaryBreakdownRow {
  category_id?: number
  category_code?: string
  category_title?: string
  incoming_cents: number
  outgoing_cents: number
}

export interface FinanceGeneratedEntrySync {
  status: 'not_synced' | 'synced'
  first_synced_at?: string
  first_synced_by?: number
  last_synced_at?: string
  last_synced_by?: number
  last_synced_reason?: string
}

export interface FinanceSummary {
  month: string
  total_incoming_cents: number
  total_outgoing_cents: number
  monthly_incoming_cents: number
  monthly_outgoing_cents: number
  monthly_net_cents: number
  property_income_cents: number
  monthly_property_income_cents: number
  cleaner_expense_cents: number
  cleaner_margin: number
  breakdown: FinanceSummaryBreakdownRow[]
  generated_entry_sync: FinanceGeneratedEntrySync
}

export interface FinanceRevenueRecognitionBooking {
  booking_id: number
  reference_number: string
  guest_name: string
  check_in_date: string
  check_out_date: string
  gross_cents: number
  stay_nights: number
  recognized_nights: number
  recognized_gross_cents: number
  unmatched: boolean
  cancelled: boolean
  no_show: boolean
}

export interface FinanceRevenueRecognitionIssue {
  booking_id: number
  reference_number: string
  guest_name: string
  check_in_date?: string
  check_out_date?: string
  reason: string
}

export interface FinanceRevenueRecognitionResponse {
  month: string
  gross_revenue_cents: number
  recognized_booking_net_cents: number
  other_incoming_cents: number
  other_outgoing_cents: number
  recognized_net_cents: number
  bookings: FinanceRevenueRecognitionBooking[]
  excluded_bookings: FinanceRevenueRecognitionIssue[]
  long_term_comparison?: FinanceLongTermComparison
}

export interface FinanceLongTermComparison {
  status: 'configured' | 'not_configured'
  rate_id: number | null
  effective_from_month: string | null
  monthly_rent_cents: number | null
  eligible_outgoing_cents: number | null
  long_term_net_cents: number | null
  short_term_difference_cents: number | null
  outcome: 'ahead' | 'behind' | 'equal' | null
}

export interface FinanceLongTermRentRate {
  id: number
  effective_from_month: string
  monthly_rent_cents: number
  currency: 'EUR'
  created_at: string
  updated_at: string
}

export interface FinanceResetDeleteCounts {
  finance_transactions: number
  finance_recurring_rules: number
  finance_bookings: number
  finance_imports: number
  finance_statement_evidence: number
  finance_booking_merges: number
  finance_month_states: number
  finance_attachment_files: number
  invoices: number
  invoice_files: number
}

export interface FinanceResetPreserveCounts {
  cleaning_salary_transactions: number
  cleaning_daily_logs: number
  cleaning_salary_adjustments: number
  cleaner_fee_history: number
  finance_categories: number
  invoice_sequences: number
  audit_logs: number
}

export interface FinanceResetRegeneratedCounts {
  cleaning_salary_inserted: number
  cleaning_salary_updated: number
}

export interface FinanceResetPreview {
  property_id: number
  would_delete: FinanceResetDeleteCounts
  would_preserve: FinanceResetPreserveCounts
}

export interface FinanceResetResult {
  ok: boolean
  deleted: FinanceResetDeleteCounts
  preserved: FinanceResetPreserveCounts
  regenerated: FinanceResetRegeneratedCounts
  reset_run_id: number
}
