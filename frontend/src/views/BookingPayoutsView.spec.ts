import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

import { api } from '@/api/http'
import BookingPayoutsView from './BookingPayoutsView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

/** jsdom's `localStorage` is read-only; replace with an in-memory shim. */
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
      name: 'Apartment A',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

/** Route API calls by URL prefix; falls back to `{ payouts: [], occupancies: [] }`. */
function apiRouter(handlers: Record<string, (opts?: { method?: string }) => unknown>) {
  apiMock.mockImplementation((url: string, opts?: { method?: string }) => {
    const match = Object.keys(handlers).find((key) => url.startsWith(key))
    if (match) return Promise.resolve(handlers[match]!(opts))
    if (url.includes('/occupancies')) return Promise.resolve({ occupancies: [] })
    if (url.includes('/booking-payouts')) return Promise.resolve({ payouts: [] })
    return Promise.resolve({})
  })
}

describe('BookingPayoutsView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the empty-state prompt when no property is selected', async () => {
    apiRouter({})
    const w = mount(BookingPayoutsView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads payouts for the active property and renders rows', async () => {
    seedProperty()
    apiRouter({
      '/api/properties/7/finance/booking-payouts': () => ({
        payouts: [
          {
            id: 1,
            reference_number: 'BK-123',
            net_cents: 12345,
            payout_date: '2026-04-10',
            guest_name: 'Jane Guest',
            check_in_date: '2026-04-01',
            check_out_date: '2026-04-05',
          },
          {
            id: 2,
            reference_number: 'BK-456',
            net_cents: 6789,
            payout_date: '2026-04-12',
          },
        ],
      }),
    })
    const w = mount(BookingPayoutsView)
    await flushPromises()
    expect(w.text()).toContain('BK-123')
    expect(w.text()).toContain('BK-456')
    expect(w.text()).toContain('Jane Guest')
    const payoutCall = apiMock.mock.calls.find(
      ([url]) => typeof url === 'string' && url.startsWith('/api/properties/7/finance/booking-payouts'),
    )
    expect(payoutCall).toBeTruthy()
  })

  it('surfaces an error banner when the payouts request fails', async () => {
    seedProperty()
    apiMock.mockImplementation((url: string) => {
      if (url.startsWith('/api/properties/7/finance/booking-payouts')) {
        return Promise.reject(new Error('upstream 503'))
      }
      return Promise.resolve({ occupancies: [] })
    })
    const w = mount(BookingPayoutsView)
    await flushPromises()
    expect(w.text()).toContain('upstream 503')
  })
})
