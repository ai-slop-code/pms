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
  last_synced_at: string
  has_payout_data?: boolean
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
}

export interface OccupancyApiToken {
  id: number
  label?: string
  created_at: string
  last_used_at?: string
}
