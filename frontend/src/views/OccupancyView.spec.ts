import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

import { api } from '@/api/http'
import OccupancyView from './OccupancyView.vue'
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

function seedProperty(id = 6) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment F',
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
    if (url.includes('/occupancies')) return Promise.resolve({ occupancies: [] })
    if (url.includes('/occupancy-sync/runs')) return Promise.resolve({ runs: [] })
    if (url.includes('/occupancy-api-tokens')) return Promise.resolve({ tokens: [] })
    if (url.includes('/occupancy-source')) {
      return Promise.resolve({ source: { active: false, source_type: '' } })
    }
    return Promise.resolve({})
  })
}

describe('OccupancyView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(OccupancyView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads the calendar tab on mount for the active property', async () => {
    seedProperty()
    apiRouter({})
    mount(OccupancyView)
    await flushPromises()
    const occCall = apiMock.mock.calls.find(
      ([u]) => typeof u === 'string' && u.includes('/occupancies'),
    )
    expect(occCall).toBeTruthy()
  })

  it('surfaces an error banner when the initial calendar load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('calendar 500'))
    const w = mount(OccupancyView)
    await flushPromises()
    expect(w.text()).toContain('calendar 500')
  })
})
