// Nuki module API types — shapes returned by /api/properties/:id/nuki/*.

export interface NukiKeypadCode {
  id: number
  external_nuki_id: string
  name?: string
  access_code_masked?: string
  valid_from?: string
  valid_until?: string
  enabled: boolean
  pms_linked: boolean
  updated_at: string
}

export interface NukiUpcomingStay {
  occupancy_id: number
  source_event_uid: string
  summary?: string
  saved_pin_name?: string
  start_at: string
  end_at: string
  occupancy_status: string
  generated_code_id?: number
  generated_label?: string
  generated_status?: string
  generated_valid_from?: string
  generated_valid_until?: string
  generated_masked?: string
  generated_error?: string
  generated_updated_at?: string
}

export interface NukiRun {
  id: number
  started_at: string
  finished_at?: string
  status: string
  trigger: string
  error_message?: string
  processed_count: number
  created_count: number
  updated_count: number
  revoked_count: number
  failed_count: number
}

/** Transient PIN reveal payload used by NukiView's inline modal. Not an API DTO. */
export interface NukiPinReveal {
  codeId: number
  pin: string
  label: string
  stayName?: string
}
