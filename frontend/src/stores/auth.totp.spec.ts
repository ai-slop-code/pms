import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from './auth'

const originalFetch = globalThis.fetch

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: 'OK',
    text: async () => JSON.stringify(body),
  } as unknown as Response
}

describe('auth store 2FA flow', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })
  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('login leaves the store in mfaPending mode when the server requests it', async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ mfa_required: true }),
    )
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const store = useAuthStore()
    await store.login('mfa@example.com', 'pw')

    expect(store.user).toBeNull()
    expect(store.mfaPending).toBe(true)
    expect(store.loaded).toBe(true)
  })

  it('verifyTwoFactor swaps mfaPending for an authenticated user', async () => {
    const fetchMock = vi.fn<typeof fetch>(async (input) => {
      const url = typeof input === 'string' ? input : (input as Request).url
      if (url.endsWith('/api/auth/2fa/verify')) {
        return jsonResponse({ user: { id: 1, email: 'mfa@example.com', role: 'owner' } })
      }
      // permission refresh after verify
      return jsonResponse({ property_permissions: [] })
    })
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const store = useAuthStore()
    store.mfaPending = true
    await store.verifyTwoFactor({ code: '123456' })

    expect(store.mfaPending).toBe(false)
    expect(store.user).toEqual({ id: 1, email: 'mfa@example.com', role: 'owner' })
  })

  it('refreshMe interprets mfa_required from /auth/me as pending', async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse({ mfa_required: true }),
    )
    globalThis.fetch = fetchMock as unknown as typeof fetch

    const store = useAuthStore()
    await store.refreshMe()

    expect(store.mfaPending).toBe(true)
    expect(store.user).toBeNull()
    expect(store.loaded).toBe(true)
  })
})
