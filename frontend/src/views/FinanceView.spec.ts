import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
  apiUrl: vi.fn((path: string) => `https://api.example.test${path}`),
}))
vi.mock('vue-router', () => ({
  RouterLink: { template: '<a><slot /></a>' },
}))
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ push: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }),
}))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

import { api } from '@/api/http'
import FinanceView from './FinanceView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

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

function seedProperty(id = 9) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment I',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

const emptySummary = {
  month: '2026-04',
  total_incoming_cents: 0,
  total_outgoing_cents: 0,
  monthly_incoming_cents: 0,
  monthly_outgoing_cents: 0,
  monthly_net_cents: 0,
  property_income_cents: 0,
  monthly_property_income_cents: 0,
  cleaner_expense_cents: 0,
  cleaner_margin: 0,
  breakdown: [],
  generated_entry_sync: {
    status: 'not_synced' as const,
  },
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/finance/categories')) return Promise.resolve({ categories: [] })
    if (url.includes('/finance/transactions')) return Promise.resolve({ transactions: [] })
    if (url.includes('/finance/summary')) return Promise.resolve(emptySummary)
    if (url.includes('/finance/recurring-rules')) return Promise.resolve({ rules: [] })
    if (url.includes('/finance/months/') && url.includes('/sync-generated')) {
      return Promise.resolve({
        ok: true,
        generated_entry_sync: {
          status: 'synced',
          first_synced_at: '2026-04-02T10:20:30Z',
          last_synced_at: '2026-04-07T09:15:00Z',
          last_synced_reason: 'manual',
        },
        changes: {
          recurring_inserted: 0,
          recurring_updated: 1,
          recurring_deleted: 0,
          cleaning_salary_inserted: 0,
          cleaning_salary_updated: 1,
        },
      })
    }
    return Promise.resolve({})
  })
}

describe('FinanceView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    document.body.innerHTML = ''
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(FinanceView, { attachTo: document.body })
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads finance data on mount and renders the summary KPIs', async () => {
    seedProperty()
    apiRouter({
      '/finance/summary': () => ({
        ...emptySummary,
        monthly_incoming_cents: 250000,
        monthly_outgoing_cents: 100000,
        monthly_net_cents: 150000,
      }),
    })
    const w = mount(FinanceView)
    await flushPromises()
    const summaryCall = apiMock.mock.calls.find(
      ([u]) => typeof u === 'string' && u.includes('/finance/summary'),
    )
    expect(summaryCall).toBeTruthy()
    expect(w.text().toLowerCase()).toContain('finance')
    expect(w.text()).toContain('Not synced')
    expect(w.text()).toContain('Sync generated entries')
  })

  it('renders synced generated-entry status from the summary', async () => {
    seedProperty()
    apiRouter({
      '/finance/summary': () => ({
        ...emptySummary,
        generated_entry_sync: {
          status: 'synced',
          first_synced_at: '2026-04-02T10:20:30Z',
          last_synced_at: '2026-04-07T09:15:00Z',
          last_synced_reason: 'manual',
        },
      }),
    })
    const w = mount(FinanceView)
    await flushPromises()
    expect(w.text()).toContain('Synced')
    expect(w.text()).toContain('Generated entries last synced')
  })

  it('calls the generated-entry sync endpoint from the toolbar', async () => {
    seedProperty()
    apiRouter({})
    const w = mount(FinanceView)
    await flushPromises()

    const syncButton = w.findAll('button').find((button) => button.text() === 'Sync generated entries')
    expect(syncButton).toBeTruthy()
    await syncButton!.trigger('click')
    await flushPromises()

    expect(
      apiMock.mock.calls.some(
        ([u, options]) =>
          typeof u === 'string' &&
          u.includes('/finance/months/') &&
          u.includes('/sync-generated') &&
          options?.method === 'POST',
      ),
    ).toBe(true)
  })

  it('previews and executes the finance reset with invoice and cleaning-salary copy', async () => {
    seedProperty()
    apiMock.mockImplementation((url: string, options?: { method?: string; json?: unknown }) => {
      if (url.includes('/finance/reset/preview')) {
        return Promise.resolve({
          property_id: 9,
          would_delete: {
            finance_transactions: 3,
            finance_recurring_rules: 1,
            finance_bookings: 2,
            finance_imports: 1,
            finance_booking_merges: 2,
            finance_month_states: 0,
            finance_attachment_files: 1,
            invoices: 1,
            invoice_files: 1,
          },
          would_preserve: {
            cleaning_salary_transactions: 1,
            cleaning_daily_logs: 4,
            cleaning_salary_adjustments: 1,
            cleaner_fee_history: 1,
            finance_categories: 10,
            invoice_sequences: 1,
            audit_logs: 0,
          },
        })
      }
      if (url.includes('/finance/reset') && options?.method === 'POST') {
        return Promise.resolve({
          ok: true,
          reset_run_id: 12,
          deleted: {
            finance_transactions: 3,
            finance_recurring_rules: 1,
            finance_bookings: 2,
            finance_imports: 1,
            finance_booking_merges: 2,
            finance_month_states: 0,
            finance_attachment_files: 1,
            invoices: 1,
            invoice_files: 1,
          },
          preserved: {
            cleaning_salary_transactions: 1,
            cleaning_daily_logs: 4,
            cleaning_salary_adjustments: 1,
            cleaner_fee_history: 1,
            finance_categories: 10,
            invoice_sequences: 1,
            audit_logs: 0,
          },
          regenerated: { cleaning_salary_inserted: 0, cleaning_salary_updated: 1 },
        })
      }
      if (url.includes('/finance/categories')) return Promise.resolve({ categories: [] })
      if (url.includes('/finance/transactions')) return Promise.resolve({ transactions: [] })
      if (url.includes('/finance/summary')) return Promise.resolve(emptySummary)
      if (url.includes('/finance/recurring-rules')) return Promise.resolve({ rules: [] })
      return Promise.resolve({})
    })

    const w = mount(FinanceView)
    await flushPromises()

    const resetToolbarButton = w.findAll('button').find((button) => button.text() === 'Reset finance records')
    expect(resetToolbarButton).toBeTruthy()
    await resetToolbarButton!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('Linked invoices')
    expect(document.body.textContent).toContain('Cleaning salary from flat entries will remain')
    expect(document.body.textContent).toContain('Invoice numbers will not be reused')

    const resetButtons = Array.from(document.body.querySelectorAll('button')).filter(
      (button) => button.textContent?.trim() === 'Reset finance records',
    )
    resetButtons[resetButtons.length - 1]!.click()
    await flushPromises()

    expect(
      apiMock.mock.calls.some(
        ([u, options]) =>
          typeof u === 'string' &&
          u.includes('/finance/reset') &&
          !u.includes('/preview') &&
          options?.method === 'POST' &&
          options?.json?.confirmed === true &&
          options?.json?.preserve_cleaning_salary === true,
      ),
    ).toBe(true)
    expect(apiMock.mock.calls.filter(([u]) => typeof u === 'string' && u.includes('/finance/summary')).length).toBeGreaterThan(1)
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('finance api down'))
    const w = mount(FinanceView)
    await flushPromises()
    expect(w.text()).toContain('finance api down')
  })
})
