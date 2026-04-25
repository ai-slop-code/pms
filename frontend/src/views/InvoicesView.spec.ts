import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

import { api } from '@/api/http'
import InvoicesView from './InvoicesView.vue'
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
    if (url.includes('/invoices/occupancy-candidates')) return Promise.resolve({ occupancies: [] })
    if (url.includes('/invoices/payout-link-candidates')) return Promise.resolve({ payouts: [] })
    if (url.includes('/invoices')) return Promise.resolve({ invoices: [] })
    return Promise.resolve({})
  })
}

describe('InvoicesView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
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
})
