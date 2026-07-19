// Nuki module API types — shapes returned by /api/properties/:id/nuki/*.

export interface NukiKeypadCode {
  id: number
  external_nuki_id: string
  account_user_id: string | null
  name: string | null
  access_code_masked: string | null
  valid_from: string | null
  valid_until: string | null
  enabled: boolean
  pms_linked: boolean
  updated_at: string
}

export interface NukiUpcomingStay {
  stay_id: number
  legacy_occupancy_id?: number
  /** @deprecated Legacy compatibility identity. */
  occupancy_id?: number
  source_event_uid: string
  summary: string | null
  saved_pin_name: string | null
  stay_type: string
  start_at: string
  end_at: string
  occupancy_status: string
  generated_code_id: number | null
  generated_label: string | null
  generated_status: string | null
  generated_valid_from: string | null
  generated_valid_until: string | null
  generated_masked: string | null
  generated_error: string | null
  generated_updated_at: string | null
}

export interface NukiRun {
  id: number
  started_at: string
  finished_at: string | null
  status: string
  trigger: string
  error_message: string | null
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
