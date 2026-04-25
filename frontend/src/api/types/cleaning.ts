// Cleaning module API types — shapes returned by /api/properties/:id/cleaning/*.

export interface CleaningLogRow {
  day_date: string
  first_entry_at?: string
  nuki_event_reference?: string
  counted_for_salary: boolean
}

export interface CleaningFeeRow {
  id: number
  cleaning_fee_amount_cents: number
  washing_fee_amount_cents: number
  effective_from: string
  created_at: string
}

export interface CleaningAdjustmentRow {
  id: number
  adjustment_amount_cents: number
  reason: string
  created_at: string
}

export interface CleaningSummary {
  month: string
  counted_days: number
  base_salary_cents: number
  adjustments_total_cents: number
  final_salary_cents: number
}

export interface CleaningHeatBucket {
  hour: number
  count: number
}

/** Minimal Nuki keypad code row used by CleaningView's cleaner-alias picker. */
export interface CleaningNukiCodeRow {
  id: number
  external_nuki_id: string
  account_user_id?: string
  name?: string
}

export interface CleaningReconcileStats {
  fetched_events: number
  auth_matched_events: number
  entry_like_events: number
  upserted_days: number
  fallback_any_event: boolean
  cleaner_alias_count: number
  requested_since_utc: string
}
