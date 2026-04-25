// Analytics module API types — /api/properties/:id/analytics/*.

export interface FreshnessResponse {
  last_ics_sync_at?: string
  last_payout_date?: string
  unmatched_payouts_count: number
  staleness_level: 'ok' | 'warn' | 'stale'
}

export interface KPIWindow {
  days: number
  nights_sold: number
  available_nights: number
  confirmed_cents: number
  estimated_cents: number
  total_revenue_cents: number
}

export interface UnsoldNight {
  date: string
  prev_guest?: string
  next_guest?: string
}

export interface CountByDay {
  date: string
  count: number
}

export interface PacePoint {
  date: string
  count: number
}

export interface OutlookResponse {
  windows: KPIWindow[]
  pacing_series: PacePoint[]
  unsold_nights: UnsoldNight[]
  new_bookings: CountByDay[]
  revenue_as_of?: string
  trailing_adr_cents: number
}

export interface PerformanceKPIs {
  nights_sold: number
  available_nights: number
  occupancy_rate: number
  adr_cents: number
  revpar_cents: number
  gross_cents: number
  net_cents: number
  commission_cents: number
  payment_fees_cents: number
  effective_take_rate: number
  matched_nights: number
}

export interface MonthlyTrendRow {
  month: string
  occupancy_rate: number
  adr_cents: number
  gross_cents: number
  nights_sold: number
  available_nights: number
}

export interface HeatmapCell {
  year: number
  week: number
  occupancy_rate: number
}

export interface DOWCell {
  dow: number
  nights_sold: number
  available_nights: number
  occupancy_rate: number
}

export interface BucketRow {
  bucket: string
  count: number
}

export interface NetPerStayRow {
  stay_id: number
  start_at: string
  end_at: string
  guest_name: string
  gross_cents: number
  commission_cents: number
  payment_fee_cents: number
  cleaning_allocated_cents: number
  net_cents: number
}

export interface CancellationStat {
  rate: number
  buckets: BucketRow[]
  total_active_plus_cancelled: number
  total_cancelled: number
}

export interface YearlyCleaningBlock {
  year: number
  series: { month: number; count: number }[]
}

export interface YearlyFinanceBlock {
  year: number
  incoming_cents: number
  outgoing_cents: number
  net_cents: number
}

export interface PerformanceResponse {
  from: string
  to: string
  kpis: PerformanceKPIs
  prior_kpis?: PerformanceKPIs
  monthly_trend: MonthlyTrendRow[]
  seasonality_heatmap: HeatmapCell[]
  dow_occupancy: DOWCell[]
  cancellation: CancellationStat
  net_per_stay: NetPerStayRow[]
  yearly_cleaning: YearlyCleaningBlock
  yearly_finance: YearlyFinanceBlock
  revenue_as_of?: string
}

export interface ADRRow {
  bucket: string
  adr_cents: number
  matched_nights: number
}

export interface GapRow {
  date: string
  prev_stay_id?: number
  next_stay_id?: number
  prev_checkout_date?: string
  next_checkin_date?: string
}

export interface ReturningStat {
  total_active: number
  returning: number
  returning_rate: number
}

export interface DemandResponse {
  from: string
  to: string
  lead_time: BucketRow[]
  length_of_stay: BucketRow[]
  adr_by_month: ADRRow[]
  adr_by_dow: ADRRow[]
  adr_by_lead_bucket: ADRRow[]
  gap_nights: GapRow[]
  orphan_midweek: GapRow[]
  returning_guests: ReturningStat
}

export interface ReturningGuestRow {
  display_name: string
  normalized: string
  stay_count: number
  total_gross_cents?: number
  first_stay: string
  last_stay: string
}

export interface ReturningGuestsResponse {
  total: number
  limit: number
  offset: number
  guests: ReturningGuestRow[]
}
