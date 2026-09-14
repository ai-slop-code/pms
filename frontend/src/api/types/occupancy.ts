// PMS 21 occupancy calendar and sync API types.

export type StayOutcome = 'cancelled_non_refundable' | 'no_show'

export interface OccupancySyncRun {
  id: number
  started_at: string
  finished_at?: string
  status: string
  error_message?: string
  events_seen: number
  http_status?: number
  trigger: string
  raw_blocks_inserted: number
  raw_blocks_updated: number
  raw_blocks_unchanged: number
  raw_blocks_deleted_from_source: number
  raw_block_conflicts: number
}

export interface OccupancyCalendarView {
  property_id: number
  month: string
  raw_blocks: CalendarRawBookingBlock[]
  named_stays: CalendarNamedStay[]
  availability_blocks: CalendarAvailabilityBlock[]
}

export interface CalendarRawBookingBlock {
  id: number
  property_id: number
  source_type: string
  source_event_uid: string
  check_in_date: string
  check_out_date: string
  status: 'active' | 'deleted_from_source' | 'conflict' | string
  raw_summary?: string
  source_dtstamp?: string
  last_sync_run_id?: number
  conflict_reason?: string
  covered_nights: string[]
  cleaning_events: CalendarCleaningEvent[]
}

export interface CalendarNamedStay {
  id: number
  property_id: number
  display_name: string
  stay_type: 'booking_com' | 'external' | 'maintenance' | 'personal_use' | string
  check_in_date: string
  check_out_date: string
  status: 'active' | 'cancelled' | 'archived' | string
  cleaning_required: boolean
  review_status: 'confirmed' | 'rejected' | string
  review_reason?: string | null
  outcome?: StayOutcome | null
  outcome_reason?: string | null
  counts_as_sold: boolean
  has_finance_evidence: boolean
  nuki_generation_status: 'not_applicable' | 'pending' | 'generated' | 'error' | string
  nuki_generation_error?: string
  covered_nights: string[]
  source_links: CalendarStaySourceLink[]
  cleaning_events: CalendarCleaningEvent[]
}

export interface CalendarCleaningEvent {
  id: number
  checkout_date: string
  cleaning_kind: 'named_stay' | string
  title: string
  status: 'pending' | 'synced' | 'error' | 'removed' | string
  google_event_id?: string
  error_message?: string
  warning_message?: string
}

export interface CalendarStaySourceLink {
  id: number
  raw_booking_block_id?: number
  source_type: string
  source_event_uid?: string
  linked_check_in_date: string
  linked_check_out_date: string
  link_status: 'active' | 'source_deleted' | 'conflict' | 'manual_unlinked' | string
  conflict_reason?: string
}

export interface CalendarAvailabilityBlock {
  id: number
  property_id: number
  block_type: 'closed' | 'off_market' | string
  start_date: string
  end_date: string
  reason?: string
  status: 'active' | 'archived' | string
  covered_nights: string[]
}
