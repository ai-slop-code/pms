import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: '42' } }),
  RouterLink: { template: '<a><slot /></a>' },
}))

const confirmMock = vi.fn()
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: confirmMock }),
}))

import { api } from '@/api/http'
import UserDetailView from './UserDetailView.vue'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

/**
 * jsdom's read-only `localStorage` throws on `setItem`, so the property
 * store's `watch(currentId, …)` fails during `fetchList`. Install an
 * in-memory shim for the duration of these specs.
 */
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

const sampleUser = { id: 42, email: 'alice@example.com', role: 'property_manager' }
const samplePerms = [
  { id: 100, property_id: 1, module: 'occupancy', permission_level: 'write' },
  { id: 101, property_id: 1, module: 'finance', permission_level: 'admin' },
]
const sampleProperties = [
  {
    id: 1,
    name: 'Apartment A',
    timezone: 'Europe/Bratislava',
    default_language: 'sk',
    owner_user_id: 1,
    active: true,
  },
]

/**
 * Routes API calls by URL so tests can set per-resource responses once,
 * without tracking call order. The handlers are stacked by insertion:
 * the most recent match wins, like `mockImplementation` but keyed by URL.
 */
function apiRouter(handlers: Record<string, (opts?: { method?: string }) => unknown>) {
  apiMock.mockImplementation((url: string, opts?: { method?: string }) => {
    const match = Object.keys(handlers).find((key) => url.startsWith(key))
    if (!match) throw new Error(`unexpected api call: ${url}`)
    return Promise.resolve(handlers[match]!(opts))
  })
}

describe('UserDetailView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
    confirmMock.mockReset()
  })

  it('loads the user + permissions + property list on mount', async () => {
    apiRouter({
      '/api/users/42': () => ({ user: sampleUser, property_permissions: samplePerms }),
      '/api/properties': () => ({ properties: sampleProperties }),
    })
    const w = mount(UserDetailView)
    await flushPromises()
    expect(w.text()).toContain('alice@example.com')
    expect(w.text()).toContain('Property Manager')
    expect(w.text()).toContain('Occupancy')
    expect(w.text()).toContain('Finance')
  })

  it('surfaces an error banner when the initial load fails', async () => {
    apiMock.mockRejectedValueOnce(new Error('forbidden'))
    const w = mount(UserDetailView)
    await flushPromises()
    expect(w.text()).toContain('forbidden')
  })

  it('removes a permission after the confirm dialog is accepted', async () => {
    apiRouter({
      '/api/users/42/property-permissions/100': () => ({}),
      '/api/users/42': () => ({ user: sampleUser, property_permissions: samplePerms }),
      '/api/properties': () => ({ properties: sampleProperties }),
    })
    confirmMock.mockResolvedValueOnce(true)
    const w = mount(UserDetailView)
    await flushPromises()
    const removeBtn = w.findAll('button').find((b) => b.text().includes('Remove'))!
    await removeBtn.trigger('click')
    await flushPromises()
    expect(confirmMock).toHaveBeenCalled()
    expect(apiMock).toHaveBeenCalledWith(
      '/api/users/42/property-permissions/100',
      expect.objectContaining({ method: 'DELETE' }),
    )
    expect(w.text()).toContain('Permission removed.')
  })

  it('does nothing when the confirm dialog is dismissed', async () => {
    apiRouter({
      '/api/users/42': () => ({ user: sampleUser, property_permissions: samplePerms }),
      '/api/properties': () => ({ properties: sampleProperties }),
    })
    confirmMock.mockResolvedValueOnce(false)
    const w = mount(UserDetailView)
    await flushPromises()
    const removeBtn = w.findAll('button').find((b) => b.text().includes('Remove'))!
    await removeBtn.trigger('click')
    await flushPromises()
    const deleteCalls = apiMock.mock.calls.filter(
      ([, opts]) => (opts as { method?: string } | undefined)?.method === 'DELETE',
    )
    expect(deleteCalls.length).toBe(0)
  })
})
