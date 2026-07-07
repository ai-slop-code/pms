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

export interface CleaningCalendarSettings {
  enabled: boolean
  calendar_id?: string
  default_duration_minutes: number
  title_prefix: string
  same_day_label: string
  no_guest_label: string
  connected_account_id?: string
  google_client_configured: boolean
  updated_at?: string
}

export interface CleaningCalendarEventRow {
  id: number
  occupancy_id: number
  google_calendar_id: string
  google_event_id?: string
  cleaning_date: string
  starts_at: string
  ends_at: string
  same_day_arrival: boolean
  next_occupancy_id?: number
  title: string
  status: 'pending' | 'synced' | 'error' | 'removed'
  warning_message?: string
  error_message?: string
  last_synced_at?: string
  updated_at: string
}

export interface CleaningCalendarReconcileStats {
  events_seen: number
  events_upserted: number
  events_removed: number
}
