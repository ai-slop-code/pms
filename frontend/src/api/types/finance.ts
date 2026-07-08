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

export interface FinanceResetDeleteCounts {
  finance_transactions: number
  finance_recurring_rules: number
  finance_bookings: number
  finance_imports: number
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
