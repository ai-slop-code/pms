import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

import { api } from '@/api/http'
import AnalyticsView from './AnalyticsView.vue'
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

function seedProperty(id = 10) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment J',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

const emptyOutlook = {
  windows: [],
  pacing_series: [],
  unsold_nights: [],
  new_bookings: [],
  trailing_adr_cents: 0,
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/analytics/freshness')) {
      return Promise.resolve({ unmatched_payouts_count: 0, staleness_level: 'ok' })
    }
    if (url.includes('/analytics/outlook')) return Promise.resolve(emptyOutlook)
    if (url.includes('/analytics/performance')) return Promise.resolve({})
    if (url.includes('/analytics/demand')) return Promise.resolve({})
    if (url.includes('/analytics/returning-guests')) return Promise.resolve({})
    return Promise.resolve({})
  })
}

describe('AnalyticsView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('does not hit the analytics API when no property is selected', async () => {
    apiRouter({})
    mount(AnalyticsView)
    await flushPromises()
    const analyticsHits = apiMock.mock.calls.filter(
      ([u]) => typeof u === 'string' && u.includes('/analytics/'),
    )
    expect(analyticsHits.length).toBe(0)
  })

  it('loads analytics on mount for the active property', async () => {
    seedProperty()
    apiRouter({})
    mount(AnalyticsView)
    await flushPromises()
    const hitAnalytics = apiMock.mock.calls.some(
      ([u]) => typeof u === 'string' && u.includes('/analytics/'),
    )
    expect(hitAnalytics).toBe(true)
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('analytics api down'))
    const w = mount(AnalyticsView)
    await flushPromises()
    expect(w.text()).toContain('analytics api down')
  })
})
