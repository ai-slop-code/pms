// Message templates / generated guest messages API types.

import type { OccupancySummary } from './occupancy'

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
  occupancy_id: number
  messages: RenderedMessage[]
  nuki_available: boolean
}

/** Re-exported for MessagesView so it has a single import site. */
export type MessagesOccupancy = OccupancySummary
