// Occupancy module API types — shapes returned by /api/properties/:id/occupancies
// and /api/properties/:id/occupancy-sync/*.

/** Full occupancy row from GET /occupancies (list view). */
export interface Occupancy {
  id: number
  source_type: string
  source_event_uid: string
  start_at: string
  end_at: string
  status: string
  raw_summary: string
  guest_display_name?: string
  last_synced_at: string
  last_sync_run_id?: number
  has_payout_data?: boolean
  // Closure / external-sale labelling (PMS_14). Absent when the row has
  // no manual label.
  closure_state?: 'closed' | 'external_sale' | null
  closure_reason?: string | null
  closure_category?: string | null
  closed_at?: string | null
  closed_by_user_id?: number | null
  external_net_amount_cents?: number | null
  external_currency?: string | null
  external_channel?: string | null
  stay_outcome?: 'cancelled_non_refundable' | 'no_show' | null
  stay_outcome_reason?: string | null
  stay_outcome_marked_at?: string | null
  stay_outcome_marked_by_user_id?: number | null
  cleaning_calendar_excluded: boolean
  cleaning_calendar_exclusion_reason?: string | null
  cleaning_calendar_excluded_at?: string | null
  cleaning_calendar_excluded_by_user_id?: number | null
  // PMS_19 durable upstream identity + night-level truth.
  upstream_source_type?: string | null
  upstream_event_uid?: string | null
  representation_kind?: string | null
  covered_nights?: string[]
  superseded?: boolean
}

/** Lightweight occupancy shape used by MessagesView's generate picker. */
export interface OccupancySummary {
  id: number
  start_at: string
  end_at: string
  status: string
  raw_summary?: string
  guest_display_name?: string
}

export interface OccupancySyncRun {
  id: number
  started_at: string
  finished_at?: string
  status: string
  error_message?: string
  events_seen: number
  occupancies_upserted: number
  http_status?: number
  trigger: string
  deletion_enabled?: boolean
  representations_deleted_from_source?: number
  duplicate_nights_resolved?: number
  named_stays_deleted_from_source?: number
}

export interface OccupancyApiToken {
  id: number
  label?: string
  created_at: string
  last_used_at?: string
}

export interface OccupancyRepairReport {
  property_id: number
  dry_run: boolean
  nights_resolved: number
  duplicates_resolved: number
  rows_deleted_from_source?: number
  resolutions?: Array<{
    local_night: string
    winner_occupancy_id: number
    winner_upstream_uid?: string
    winner_kind?: string
    loser_occupancy_ids: number[]
    reason: string
  }>
  row_actions?: Array<{
    occupancy_id: number
    upstream_uid: string
    action: string
    reason: string
    guest_name?: string
    revoke_nuki: boolean
    remove_cleaning: boolean
  }>
}
