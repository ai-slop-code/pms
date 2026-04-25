// Invoice module API types — shapes returned by /api/properties/:id/invoices/*.

export interface InvoiceParty {
  name: string
  company_name?: string
  address_line_1?: string
  city?: string
  postal_code?: string
  country?: string
  ico?: string
  dic?: string
  vat_id?: string
}

export interface InvoiceFile {
  id: number
  version: number
  file_path: string
  file_size_bytes: number
  created_at: string
}

export interface Invoice {
  id: number
  occupancy_id?: number
  booking_payout_id?: number
  invoice_number: string
  sequence_year: number
  sequence_value: number
  language: 'sk' | 'en'
  issue_date: string
  taxable_supply_date: string
  due_date: string
  stay_start_date: string
  stay_end_date: string
  supplier: InvoiceParty
  customer: InvoiceParty
  amount_total_cents: number
  currency: string
  payment_status: string
  payment_note: string
  version: number
  latest_file_path?: string
  latest_file_size_bytes?: number
  latest_file_created_at?: string
  download_url: string
  files?: InvoiceFile[]
  created_at: string
  updated_at: string
}

export interface InvoicePreview {
  year: number
  sequence_value: number
  invoice_number: string
}

/**
 * Occupancy candidate shown in the InvoicesView "link stay" picker.
 * Different shape from {@link ./bookingPayouts.OccupancyOption} and
 * {@link ./occupancy.Occupancy} — kept distinct on purpose.
 */
export interface InvoiceOccupancyOption {
  id: number
  start_at: string
  end_at: string
  status: string
  summary: string
  guest_display_name?: string
  has_payout_data: boolean
}

export interface InvoiceBookingPayoutOption {
  id: number
  reference_number: string
  net_cents: number
  amount_cents?: number | null
  check_in_date?: string | null
  check_out_date?: string | null
  guest_name?: string | null
  host_name?: string | null
  payout_summary?: string | null
  occupancy_id?: number | null
  occupancy_summary?: string | null
  linked_invoice_id?: number | null
}
