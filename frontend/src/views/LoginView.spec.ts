import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

const pushMock = vi.fn()
const replaceMock = vi.fn()
const routeQuery: { query: Record<string, string> } = { query: {} }

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock, replace: replaceMock }),
  useRoute: () => routeQuery,
}))

import { api } from '@/api/http'
import LoginView from './LoginView.vue'
import { useAuthStore } from '@/stores/auth'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

function mountView() {
  return mount(LoginView)
}

describe('LoginView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
    pushMock.mockReset()
    replaceMock.mockReset()
    routeQuery.query = {}
  })

  it('renders the sign-in form with email and password fields', () => {
    const w = mountView()
    expect(w.text()).toContain('Sign in')
    expect(w.find('input[type="email"]').exists()).toBe(true)
    expect(w.find('input[type="password"]').exists()).toBe(true)
  })

  it('submits credentials via the auth store and redirects to the default route', async () => {
    apiMock.mockImplementation((url: string) => {
      if (url === '/api/auth/login') {
        return Promise.resolve({ user: { id: 1, email: 'a@b.c', role: 'owner' } })
      }
      return Promise.resolve({ property_permissions: [] })
    })
    const w = mountView()
    await w.find('input[type="email"]').setValue('a@b.c')
    await w.find('input[type="password"]').setValue('s3cret')
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    const auth = useAuthStore()
    expect(auth.user?.email).toBe('a@b.c')
    expect(replaceMock).toHaveBeenCalledWith('/')
  })

  it('honours the ?redirect= query parameter after a successful login', async () => {
    routeQuery.query = { redirect: '/analytics' }
    apiMock.mockImplementation((url: string) => {
      if (url === '/api/auth/login') {
        return Promise.resolve({ user: { id: 1, email: 'a@b.c', role: 'owner' } })
      }
      return Promise.resolve({ property_permissions: [] })
    })
    const w = mountView()
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(replaceMock).toHaveBeenCalledWith('/analytics')
  })

  it('surfaces an error banner when the API rejects', async () => {
    apiMock.mockRejectedValueOnce(new Error('Invalid credentials'))
    const w = mountView()
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(w.text()).toContain('Invalid credentials')
    expect(replaceMock).not.toHaveBeenCalled()
  })

  it('falls back to a generic message when the rejection is not an Error', async () => {
    apiMock.mockRejectedValueOnce('boom')
    const w = mountView()
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(w.text()).toContain('Login failed')
  })
})
