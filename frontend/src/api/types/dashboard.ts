// Dashboard widget API types — GET /api/properties/:id/dashboard.

export interface SyncStatusWidget {
  occupancy?: string
  nuki?: string
}

export interface UpcomingStayWidget {
  occupancy_id: number
  summary?: string
  start_at: string
  end_at: string
  status: string
}

export interface ActiveNukiCodeWidget {
  occupancy_id: number
  summary?: string
  code_label?: string
  code_masked?: string
  status: string
  valid_from?: string
  valid_until?: string
  last_updated_at?: string
  error_message?: string
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
  customer_name?: string
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
