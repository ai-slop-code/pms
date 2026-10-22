import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
  apiUrl: vi.fn((path: string) => `https://api.example.test${path}`),
}))

import { api, apiUrl } from '@/api/http'
import InvoicesView from './InvoicesView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>
const apiUrlMock = apiUrl as unknown as ReturnType<typeof vi.fn>

function installLocalStorageStub() {
  const store = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      get length() {
        return store.size
      },
      clear: () => store.clear(),
      getItem: (k: string) => (store.has(k) ? (store.get(k) as string) : null),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      removeItem: (k: string) => {
        store.delete(k)
      },
      setItem: (k: string, v: string) => {
        store.set(k, v)
      },
    } satisfies Storage,
    configurable: true,
    writable: true,
  })
}

function seedProperty(id = 7) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment G',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/invoice-sequence/next-preview')) {
      return Promise.resolve({ next_number: 'INV-2026-0001', year: 2026, sequence: 1 })
    }
    if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [] })
    if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
    if (url.includes('/invoices')) return Promise.resolve({ invoices: [] })
    return Promise.resolve({})
  })
}

function invoiceFixture(amount_total_cents: number, id = 101) {
  return {
    id,
    named_stay_id: 55,
    booking_payout_id: null,
    invoice_number: `INV-2026-${String(id).padStart(4, '0')}`,
    sequence_year: 2026,
    sequence_value: id,
    language: 'sk' as const,
    issue_date: '2026-04-15T00:00:00Z',
    taxable_supply_date: '2026-04-15T00:00:00Z',
    due_date: '2026-04-15T00:00:00Z',
    stay_start_date: '2026-04-01T00:00:00Z',
    stay_end_date: '2026-04-02T00:00:00Z',
    supplier: {},
    customer: { name: 'Guest' },
    amount_total_cents,
    currency: 'EUR',
    payment_status: 'paid',
    payment_note: 'Already paid via Booking.com.',
    version: 1,
    download_url: `/api/properties/7/invoices/${id}/download`,
    created_at: '2026-04-15T00:00:00Z',
    updated_at: '2026-04-15T00:00:00Z',
  }
}

const namedStay = {
  id: 55,
  display_name: 'Manual stay',
  stay_type: 'external' as const,
  check_in_date: '2026-04-01',
  check_out_date: '2026-04-02',
  status: 'completed',
  has_finance_data: false,
}

describe('InvoicesView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
    apiUrlMock.mockClear()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(InvoicesView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads invoices and renders a row from the API', async () => {
    seedProperty()
    apiRouter({
      '/invoices': () => ({
        invoices: [
          {
            id: 101,
            invoice_number: 'INV-2026-0007',
            issue_date: '2026-04-15',
            total_cents: 12500,
            currency: 'EUR',
            customer: { company_name: 'ACME s.r.o.' },
            status: 'issued',
            version: 1,
          },
        ],
      }),
    })
    const w = mount(InvoicesView)
    await flushPromises()
    expect(w.text()).toContain('INV-2026-0007')
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('invoices api down'))
    const w = mount(InvoicesView)
    await flushPromises()
    expect(w.text()).toContain('invoices api down')
  })

  it('loads named-stay invoice candidates from the stay endpoint', async () => {
    seedProperty()
    apiRouter({})
    mount(InvoicesView)
    await flushPromises()

    expect(
      apiMock.mock.calls.some(([url]) => String(url).includes('/invoices/stay-candidates?limit=120')),
    ).toBe(true)
    expect(apiMock.mock.calls.some(([url]) => String(url).includes('/invoices/occupancy-candidates'))).toBe(
      false,
    )
  })

  it('requires a named stay before creating an invoice', async () => {
    seedProperty()
    apiRouter({})
    const w = mount(InvoicesView)
    await flushPromises()

    const vm = w.vm as unknown as Record<string, unknown>
    await (vm.saveInvoice as () => Promise<void>)()

    expect(w.text()).toContain('Select a named stay.')
    expect(
      apiMock.mock.calls.filter(
        ([url, options]) => String(url).endsWith('/invoices') && options?.method === 'POST',
      ),
    ).toHaveLength(0)
  })

  it.each([
    [123.99, 12399],
    [4234.23, 423423],
  ])('creates a manual invoice with %s EUR as %s cents', async (amount, cents) => {
    seedProperty()
    const created = invoiceFixture(cents)
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [namedStay] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
      if (url.endsWith('/invoices') && options?.method === 'POST') return Promise.resolve({ invoice: created })
      if (url.endsWith('/invoices/101')) return Promise.resolve({ invoice: created })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [] })
      if (url.includes('/invoice-sequence/next-preview')) {
        return Promise.resolve({ next_number: 'INV-2026-0001', year: 2026, sequence: 1 })
      }
      return Promise.resolve({})
    })

    const w = mount(InvoicesView)
    await flushPromises()
    await w.find('select').setValue('55')
    const amountInput = w.find('input[step="0.01"]')
    await amountInput.setValue(String(amount))
    expect((amountInput.element as HTMLInputElement).validity.stepMismatch).toBe(false)
    await w.find('form').trigger('submit')
    await flushPromises()

    const createCall = apiMock.mock.calls.find(
      ([url, options]) => String(url).endsWith('/invoices') && options?.method === 'POST',
    )
    expect(createCall?.[1].json).toMatchObject({ amount_total_cents: cents, named_stay_id: 55 })
    expect(createCall?.[1].json.booking_payout_id).toBeUndefined()
  })

  it('accepts a fractional payout prefill and sends its exact cents', async () => {
    seedProperty()
    const payout = {
      id: 88,
      reference_number: 'PAY-88',
      net_cents: 423423,
      amount_cents: 423423,
      named_stay_id: 55,
      check_in_date: '2026-04-01',
      check_out_date: '2026-04-02',
      guest_name: 'Guest',
    }
    const created = invoiceFixture(423423)
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [namedStay] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [payout] })
      if (url.endsWith('/invoices') && options?.method === 'POST') return Promise.resolve({ invoice: created })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [] })
      if (url.includes('/invoice-sequence/next-preview')) {
        return Promise.resolve({ next_number: 'INV-2026-0001', year: 2026, sequence: 1 })
      }
      return Promise.resolve({})
    })

    const w = mount(InvoicesView)
    await flushPromises()
    await w.findAll('select')[1]!.setValue('88')
    const amountInput = w.find('input[step="0.01"]')
    expect((amountInput.element as HTMLInputElement).value).toBe('4234.23')
    expect((amountInput.element as HTMLInputElement).validity.stepMismatch).toBe(false)
    await w.find('form').trigger('submit')
    await flushPromises()

    const createCall = apiMock.mock.calls.find(
      ([url, options]) => String(url).endsWith('/invoices') && options?.method === 'POST',
    )
    expect(createCall?.[1].json).toMatchObject({ amount_total_cents: 423423, booking_payout_id: 88 })
  })

  it('enters a pro forma reservation reference without a payout', async () => {
    seedProperty()
    const created = { ...invoiceFixture(123456), document_type: 'proforma' as const, booking_reference: '6429185079' }
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [{ ...namedStay, stay_type: 'booking_com' }] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
      if (url.endsWith('/invoices') && options?.method === 'POST') return Promise.resolve({ invoice: created })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [] })
      if (url.includes('/invoice-sequence/next-preview')) return Promise.resolve({ invoice_number: 'PF/P007/2026/0001', year: 2026, sequence_value: 1, document_type: 'proforma' })
      return Promise.resolve({})
    })
    const w = mount(InvoicesView)
    await flushPromises()
    await w.findAll('select')[0]!.setValue('55')
    await w.findAll('select')[2]!.setValue('proforma')
    await w.find('input[placeholder="6429185079"]').setValue('6429185079')
    await w.find('input[step="0.01"]').setValue('1234.56')
    await w.find('form').trigger('submit')
    await flushPromises()

    const createCall = apiMock.mock.calls.find(
      ([url, options]) => String(url).endsWith('/invoices') && options?.method === 'POST',
    )
    expect(createCall?.[1].json).toMatchObject({
      document_type: 'proforma',
      booking_reference: '6429185079',
      amount_total_cents: 123456,
    })
    expect(createCall?.[1].json.booking_payout_id).toBeUndefined()
    expect(createCall?.[1].json.taxable_supply_date).toBeUndefined()
  })

  it('edits a fractional invoice through the form and preserves cents', async () => {
    seedProperty()
    const original = invoiceFixture(12399)
    const updated = invoiceFixture(423423)
    let saved = original
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [namedStay] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
      if (url.endsWith('/invoices/101') && options?.method === 'PATCH') {
        saved = updated
        return Promise.resolve({ invoice: saved })
      }
      if (url.endsWith('/invoices/101')) return Promise.resolve({ invoice: saved })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [saved] })
      if (url.includes('/invoice-sequence/next-preview')) {
        return Promise.resolve({ next_number: 'INV-2026-0001', year: 2026, sequence: 1 })
      }
      return Promise.resolve({})
    })

    const w = mount(InvoicesView)
    await flushPromises()
    await w.find('.invoice-list-item').trigger('click')
    await flushPromises()
    const amountInput = w.find('input[step="0.01"]')
    await amountInput.setValue('4234.23')
    await w.find('form').trigger('submit')
    await flushPromises()

    const updateCall = apiMock.mock.calls.find(
      ([url, options]) => String(url).endsWith('/invoices/101') && options?.method === 'PATCH',
    )
    expect(updateCall?.[1].json).toMatchObject({ amount_total_cents: 423423 })
    expect((amountInput.element as HTMLInputElement).value).toBe('4234.23')
  })

  it('adds fractional cleaning, preserves it on payout selection, and removes a row', async () => {
    seedProperty()
    const payout = { id: 88, reference_number: 'PAY-88', net_cents: 100000, amount_cents: 100000, named_stay_id: 55 }
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [namedStay] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [payout] })
      if (url.endsWith('/invoices') && options?.method === 'POST') return Promise.resolve({ invoice: invoiceFixture(100152) })
      if (url.endsWith('/invoices/101')) return Promise.resolve({ invoice: invoiceFixture(100152) })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [] })
      return Promise.resolve({})
    })
    const w = mount(InvoicesView)
    await flushPromises()
    await w.findAll('button').find((button) => button.text() === 'Add item')!.trigger('click')
    const cleaning = w.findAll('[data-testid="invoice-item"]')[1]!
    await cleaning.find('input[type="text"]').setValue('Cleaning')
    await cleaning.find('input[step="0.000001"]').setValue('1.5')
    await cleaning.find('input[step="0.01"]').setValue('1.01')
    expect((cleaning.find('input[step="0.000001"]').element as HTMLInputElement).validity.stepMismatch).toBe(false)
    await w.findAll('select')[1]!.setValue('88')
    expect(cleaning.find('input[step="0.01"]').element).toHaveProperty('value', '1.01')
    await w.findAll('button').find((button) => button.text() === 'Add item')!.trigger('click')
    await w.find('button[aria-label="Remove item 3"]').trigger('click')
    expect(w.findAll('[data-testid="invoice-item"]')).toHaveLength(2)
    await w.find('form').trigger('submit')
    await flushPromises()
    const call = apiMock.mock.calls.find(([, options]) => options?.method === 'POST')
    expect(call?.[1].json).toMatchObject({
      amount_total_cents: 100152,
      items: [
        { quantity: '1', unit_price_cents: 100000, line_total_cents: 100000 },
        { description: 'Cleaning', quantity: '1.5', unit_price_cents: 101, line_total_cents: 152 },
      ],
    })
  })

  it('copies saved pro forma items and customer into an independent final draft', async () => {
    seedProperty()
    const source = {
      ...invoiceFixture(100152), document_type: 'proforma',
      taxable_supply_date: null, due_date: null, payment_status: 'not_applicable', payment_note: '',
      supplier: { vat_status: 'non_vat_payer' },
      customer: { name: 'Guest', company_name: 'Company', address_line_1: 'Main 1' },
      booking_reference: 'REF-55', guest_name: 'Guest', price_details: 'Accommodation and cleaning',
      stay_start_date: '2026-03-31T22:00:00Z', stay_end_date: '2026-04-01T22:00:00Z',
      items: [
        { description: 'Accommodation', quantity: '1', unit_price_cents: 100000, line_total_cents: 100000 },
        { description: 'Cleaning', quantity: '1.5', unit_price_cents: 101, line_total_cents: 152 },
      ],
    }
    apiMock.mockImplementation((url: string, options?: { method?: string }) => {
      if (url.includes('/invoices/stay-candidates')) return Promise.resolve({ stays: [namedStay] })
      if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
      if (url.endsWith('/invoices') && options?.method === 'POST') return Promise.resolve({ invoice: { ...source, id: 102, document_type: 'invoice' } })
      if (url.endsWith('/invoices/101')) return Promise.resolve({ invoice: source })
      if (url.endsWith('/invoices/102')) return Promise.resolve({ invoice: { ...source, id: 102, document_type: 'invoice' } })
      if (url.endsWith('/invoices')) return Promise.resolve({ invoices: [source] })
      return Promise.resolve({})
    })
    const w = mount(InvoicesView)
    await flushPromises()
    await w.find('.invoice-list-item').trigger('click')
    await flushPromises()
    // Unsaved edits must not become the source of the copied draft.
    await w.findAll('input[step="0.01"]')[1]!.setValue('9.99')
    await w.findAll('button').find((button) => button.text() === 'Create final invoice')!.trigger('click')
    await flushPromises()
    expect(w.findAll('input[step="0.01"]')[1]!.element).toHaveProperty('value', '1.01')
    expect(apiMock.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
    expect(w.text()).toContain('Final invoice draft copied')
    expect(w.text()).not.toContain('Regenerate PDF')
    await w.findAll('input[step="0.000001"]')[1]!.setValue('2.5')
    await w.find('form').trigger('submit')
    await flushPromises()
    const call = apiMock.mock.calls.find(([, options]) => options?.method === 'POST')
    expect(call?.[1].json).toMatchObject({
      document_type: 'invoice', named_stay_id: 55, amount_total_cents: 100253,
      stay_start_date: '2026-04-01', stay_end_date: '2026-04-02',
      booking_reference: 'REF-55', supplier_vat_status: 'non_vat_payer',
      customer: { company_name: 'Company' },
    })
    expect(call?.[1].json.taxable_supply_date).toBeTruthy()
    expect(source.items[1]!.quantity).toBe('1.5')
    expect(source.items[1]!.unit_price_cents).toBe(101)
  })

  it('opens an existing final invoice instead of creating another draft', async () => {
    seedProperty()
    const source = { ...invoiceFixture(10000), document_type: 'proforma' }
    const final = { ...invoiceFixture(10000, 102), document_type: 'invoice' }
    apiRouter({
      '/invoices/101': () => ({ invoice: source }),
      '/invoices/102': () => ({ invoice: final }),
      '/invoices': () => ({ invoices: [source, final] }),
    })
    const w = mount(InvoicesView)
    await flushPromises()
    await w.find('.invoice-list-item').trigger('click')
    await flushPromises()
    await w.findAll('button').find((button) => button.text() === 'Open final invoice')!.trigger('click')
    await flushPromises()
    expect(w.text()).toContain('Edit invoice')
    expect(w.text()).not.toContain('Create final invoice')
    expect(apiMock.mock.calls.some(([, options]) => options?.method === 'POST')).toBe(false)
  })

  it('downloads invoices through the configured API origin', async () => {
    seedProperty()
    const invoice = {
      id: 101,
      named_stay_id: 55,
      booking_payout_id: null,
      invoice_number: 'INV-2026-0007',
      sequence_year: 2026,
      sequence_value: 7,
      language: 'sk',
      issue_date: '2026-04-15T00:00:00Z',
      taxable_supply_date: '2026-04-15T00:00:00Z',
      due_date: '2026-04-15T00:00:00Z',
      stay_start_date: '2026-04-01T00:00:00Z',
      stay_end_date: '2026-04-02T00:00:00Z',
      supplier: {},
      customer: { company_name: 'ACME s.r.o.' },
      amount_total_cents: 12500,
      currency: 'EUR',
      payment_status: 'paid',
      payment_note: 'Already paid via Booking.com.',
      version: 1,
      latest_file_path: 'invoices/inv.pdf',
      latest_file_size_bytes: 1234,
      latest_file_created_at: '2026-04-15T00:00:00Z',
      download_url: '/api/properties/7/invoices/101/download',
      created_at: '2026-04-15T00:00:00Z',
      updated_at: '2026-04-15T00:00:00Z',
    }
    apiRouter({
      '/invoices/101': () => ({ invoice }),
      '/invoices': () => ({ invoices: [invoice] }),
    })
    const w = mount(InvoicesView)
    await flushPromises()

    await w.find('.invoice-list-item').trigger('click')
    await flushPromises()
    const downloadButton = w.findAll('button').find((button) => button.text() === 'Download PDF')
    expect(downloadButton).toBeFalsy()
    const downloadLink = w.find('a.ui-btn')

    expect(apiUrlMock).toHaveBeenCalledWith('/api/properties/7/invoices/101/download')
    expect(downloadLink.attributes('href')).toBe(
      'https://api.example.test/api/properties/7/invoices/101/download',
    )
  })
})
