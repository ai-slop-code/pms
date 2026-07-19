// Dashboard widget API types — GET /api/properties/:id/dashboard.

export interface SyncStatusWidget {
  occupancy?: string
  nuki?: string
}

export interface UpcomingStayWidget {
  stay_id: number
  occupancy_id?: number
  summary: string | null
  start_at: string
  end_at: string
  status: string
}

export interface ActiveNukiCodeWidget {
  nuki_code_id: number
  stay_id: number
  /** @deprecated Legacy compatibility identity. */
  occupancy_id?: number
  summary: string | null
  code_label: string | null
  code_masked: string | null
  status: string
  valid_from: string | null
  valid_until: string | null
  last_updated_at: string | null
  error_message: string | null
}

export interface CleaningMonthWidget {
  counted_days: number
  salary_draft: number
}

export interface FinanceMonthWidget {
  incoming: number
  outgoing: number
  net: number
}

export interface RecentInvoiceWidget {
  invoice_id: number
  invoice_number: string
  customer_name: string | null
  amount_total_cents: number
  issue_date: string
  version: number
}

export interface DashboardWidgets {
  sync_status?: SyncStatusWidget
  upcoming_stays?: UpcomingStayWidget[]
  active_nuki_codes?: ActiveNukiCodeWidget[]
  cleaning_month?: CleaningMonthWidget
  finance_month?: FinanceMonthWidget
  recent_invoices?: RecentInvoiceWidget[]
}
