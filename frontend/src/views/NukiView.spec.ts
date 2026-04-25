import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))
vi.mock('@/composables/useToast', () => ({ useToast: () => ({ push: vi.fn() }) }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

import { api } from '@/api/http'
import NukiView from './NukiView.vue'
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

function seedProperty(id = 5) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment E',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

/** Route API calls by URL substring with empty-shaped fallbacks for every endpoint the view may hit. */
function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/nuki/codes')) return Promise.resolve({ codes: [] })
    if (url.includes('/nuki/upcoming-stays')) return Promise.resolve({ stays: [] })
    if (url.includes('/nuki/runs')) return Promise.resolve({ runs: [], has_more: false })
    return Promise.resolve({})
  })
}

describe('NukiView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(NukiView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads nuki data on mount and renders stay rows', async () => {
    seedProperty()
    apiRouter({
      '/nuki/upcoming-stays': () => ({
        stays: [
          {
            occupancy_id: 77,
            source_event_uid: 'uid-77',
            summary: 'Guest Smith',
            start_at: '2026-05-01T14:00:00Z',
            end_at: '2026-05-04T10:00:00Z',
            occupancy_status: 'active',
          },
        ],
      }),
    })
    const w = mount(NukiView)
    await flushPromises()
    expect(w.text()).toContain('May 1, 2026')
    expect(apiMock).toHaveBeenCalled()
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('nuki api down'))
    const w = mount(NukiView)
    await flushPromises()
    expect(w.text()).toContain('nuki api down')
  })
})
