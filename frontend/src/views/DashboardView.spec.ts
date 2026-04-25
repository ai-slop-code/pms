import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

vi.mock('vue-router', () => ({
  RouterLink: { template: '<a><slot /></a>' },
}))

import { api } from '@/api/http'
import DashboardView from './DashboardView.vue'
import { usePropertyStore } from '@/stores/property'
import { useAuthStore } from '@/stores/auth'

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

function seedProperty(id = 4) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment D',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  const auth = useAuthStore()
  auth.user = { id: 1, email: 'a@b.c', role: 'super_admin' }
  return id
}

describe('DashboardView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the select-property prompt when no property is active', async () => {
    const w = mount(DashboardView)
    await flushPromises()
    expect(w.text()).toContain('Select a property to load the dashboard.')
    expect(apiMock).not.toHaveBeenCalled()
  })

  it('loads the dashboard widgets for the active property and renders them', async () => {
    seedProperty()
    apiMock.mockResolvedValue({
      widgets: {
        sync_status: { occupancy: 'ok', nuki: 'ok' },
        upcoming_stays: [
          {
            occupancy_id: 11,
            summary: 'Jane Guest',
            start_at: '2026-05-01T14:00:00Z',
            end_at: '2026-05-04T10:00:00Z',
            status: 'confirmed',
          },
        ],
        cleaning_month: { counted_days: 3, salary_draft: 7500 },
        finance_month: { incoming: 100000, outgoing: 25000, net: 75000 },
        recent_invoices: [],
        active_nuki_codes: [],
      },
    })
    const w = mount(DashboardView)
    await flushPromises()
    expect(apiMock).toHaveBeenCalledWith('/api/properties/4/dashboard')
    expect(w.text()).toContain('Jane Guest')
  })

  it('surfaces an error banner when the dashboard endpoint rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('internal server error'))
    const w = mount(DashboardView)
    await flushPromises()
    expect(w.text()).toContain('internal server error')
  })
})
