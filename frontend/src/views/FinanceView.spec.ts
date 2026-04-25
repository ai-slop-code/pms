import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))
vi.mock('vue-router', () => ({
  RouterLink: { template: '<a><slot /></a>' },
}))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
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
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/finance/categories')) return Promise.resolve({ categories: [] })
    if (url.includes('/finance/transactions')) return Promise.resolve({ transactions: [] })
    if (url.includes('/finance/summary')) return Promise.resolve(emptySummary)
    if (url.includes('/finance/recurring-rules')) return Promise.resolve({ rules: [] })
    return Promise.resolve({})
  })
}

describe('FinanceView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(FinanceView)
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
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('finance api down'))
    const w = mount(FinanceView)
    await flushPromises()
    expect(w.text()).toContain('finance api down')
  })
})
