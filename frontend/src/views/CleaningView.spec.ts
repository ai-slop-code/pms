import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

import { api } from '@/api/http'
import CleaningView from './CleaningView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

/** jsdom's `localStorage` is read-only; swap in an in-memory shim. */
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

function seedProperty(id = 3) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment C',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

/**
 * The view fires seven parallel `Promise.all` calls on mount. Route each by
 * URL substring so tests only describe the pieces they care about. Anything
 * unmatched returns an empty-shaped response the view treats as harmless.
 */
function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/cleaning/logs')) return Promise.resolve({ logs: [] })
    if (url.includes('/cleaning/summary')) {
      return Promise.resolve({
        month: '',
        counted_days: 0,
        base_salary_cents: 0,
        adjustments_total_cents: 0,
        final_salary_cents: 0,
      })
    }
    if (url.includes('/cleaning/heatmap')) return Promise.resolve({ buckets: [] })
    if (url.includes('/cleaning/fees')) return Promise.resolve({ fees: [] })
    if (url.includes('/cleaning/adjustments')) return Promise.resolve({ adjustments: [] })
    if (url.includes('/settings')) return Promise.resolve({ profile: {} })
    if (url.includes('/nuki/codes')) return Promise.resolve({ codes: [] })
    return Promise.resolve({})
  })
}

describe('CleaningView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(CleaningView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads cleaning data for the active property and renders summary + logs', async () => {
    seedProperty()
    apiRouter({
      '/cleaning/logs': () => ({
        logs: [
          { day_date: '2026-04-02', first_entry_at: '2026-04-02T10:15:00Z', counted_for_salary: true },
          { day_date: '2026-04-05', counted_for_salary: false },
        ],
      }),
      '/cleaning/summary': () => ({
        month: '2026-04',
        counted_days: 1,
        base_salary_cents: 2500,
        adjustments_total_cents: 0,
        final_salary_cents: 2500,
      }),
    })
    const w = mount(CleaningView)
    await flushPromises()
    expect(w.text()).toContain('2026-04-02')
    expect(w.text()).toContain('2026-04-05')
    // Seven GET calls on mount.
    expect(apiMock.mock.calls.length).toBeGreaterThanOrEqual(7)
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('upstream 500'))
    const w = mount(CleaningView)
    await flushPromises()
    expect(w.text()).toContain('upstream 500')
  })
})
