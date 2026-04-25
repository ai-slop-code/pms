import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { nextTick, reactive } from 'vue'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

const pushMock = vi.fn()
const replaceMock = vi.fn()
const routeStub = reactive<{
  name: string
  path: string
  fullPath: string
  meta: Record<string, unknown>
}>({
  name: 'dashboard',
  path: '/',
  fullPath: '/',
  meta: {},
})
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock, replace: replaceMock }),
  useRoute: () => routeStub,
  RouterView: { template: '<div data-test="router-view" />' },
}))

vi.mock('@/components/shell/AppTopbar.vue', () => ({
  default: {
    name: 'AppTopbar',
    emits: ['toggle-sidebar', 'logout'],
    template:
      '<header><button data-test="logout" @click="$emit(\'logout\')">Logout</button></header>',
  },
}))
vi.mock('@/components/shell/AppSidebar.vue', () => ({
  default: { name: 'AppSidebar', template: '<nav />' },
}))
vi.mock('@/components/ui/ToastStack.vue', () => ({
  default: { name: 'ToastStack', template: '<div />' },
}))
vi.mock('@/components/ui/ConfirmHost.vue', () => ({
  default: { name: 'ConfirmHost', template: '<div />' },
}))

vi.mock('@/composables/useDocumentTitle', () => ({
  useDocumentTitle: vi.fn(),
}))

import { api } from '@/api/http'
import ShellView from './ShellView.vue'
import { useAuthStore } from '@/stores/auth'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

/**
 * jsdom's `localStorage` is read-only; the property store writes into it
 * on `loadStored`/`watch(currentId)`. Swap in an in-memory shim.
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

beforeEach(() => {
  installLocalStorageStub()
  setActivePinia(createPinia())
  apiMock.mockReset()
  pushMock.mockReset()
  replaceMock.mockReset()
  Object.assign(routeStub, { name: 'dashboard', path: '/', fullPath: '/', meta: {} })
})

describe('ShellView', () => {
  it('renders the app shell scaffolding', () => {
    apiMock.mockResolvedValue({ properties: [] })
    const w = mount(ShellView)
    expect(w.find('[data-test="router-view"]').exists()).toBe(true)
    expect(w.find('#main-content').exists()).toBe(true)
  })

  it('logs out and navigates to /login when the topbar emits logout', async () => {
    apiMock.mockResolvedValue({ properties: [] })
    const auth = useAuthStore()
    const logoutSpy = vi.spyOn(auth, 'logout').mockResolvedValue(undefined)
    const w = mount(ShellView)
    await w.find('[data-test="logout"]').trigger('click')
    await flushPromises()
    expect(logoutSpy).toHaveBeenCalled()
    expect(pushMock).toHaveBeenCalledWith('/login')
  })

  it('fetches the property list on mount when the user is already signed in', async () => {
    const auth = useAuthStore()
    auth.user = { id: 7, email: 'a@b.c', role: 'super_admin' }
    apiMock.mockResolvedValue({ properties: [] })
    mount(ShellView)
    await flushPromises()
    expect(apiMock).toHaveBeenCalledWith('/api/properties')
  })

  it('redirects to / when the active route requires a module the user cannot access', async () => {
    const auth = useAuthStore()
    auth.user = { id: 1, email: 'x@y.z', role: 'read_only' }
    auth.propertyPermissions = []
    apiMock.mockResolvedValue({
      properties: [
        {
          id: 5,
          name: 'P',
          timezone: 'Europe/Bratislava',
          default_language: 'sk',
          owner_user_id: 1,
          active: true,
        },
      ],
    })
    routeStub.name = 'finance'
    routeStub.path = '/finance'
    routeStub.fullPath = '/finance'
    routeStub.meta = { module: 'finance' }
    mount(ShellView)
    await flushPromises()
    // Trigger the watcher by nudging fullPath.
    routeStub.fullPath = '/finance?x=1'
    await nextTick()
    await flushPromises()
    expect(replaceMock).toHaveBeenCalledWith('/')
  })

  it('does not redirect from /login even without module access', async () => {
    apiMock.mockResolvedValue({ properties: [] })
    routeStub.name = 'login'
    routeStub.path = '/login'
    routeStub.fullPath = '/login'
    routeStub.meta = { module: 'finance' }
    mount(ShellView)
    await flushPromises()
    expect(replaceMock).not.toHaveBeenCalled()
  })
})
