// Message templates / generated guest messages API types.

export interface MessageTemplate {
  id: number
  property_id: number
  language_code: string
  template_type: string
  title: string
  body: string
  active: boolean
  created_at: string
  updated_at: string
}

export interface CleaningMessageResponse {
  language_code: string
  title: string
  body: string
  stays_count: number
}

export interface RenderedMessage {
  language_code: string
  title: string
  body: string
  nuki_available: boolean
}

export interface GenerateMessagesResponse {
  stay_id: number
  occupancy_id?: number
  messages: RenderedMessage[]
  nuki_available: boolean
}

export interface MessagesStay {
  id: number
  display_name: string
  stay_type: 'booking_com' | 'external' | 'maintenance' | 'personal_use'
  check_in_date: string
  check_out_date: string
  nuki_status: string
}
