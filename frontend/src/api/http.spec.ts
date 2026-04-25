import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { api } from './http'

const originalFetch = globalThis.fetch

function mockFetch(response: { ok: boolean; status: number; body: string }) {
  globalThis.fetch = vi.fn(async () => {
    return {
      ok: response.ok,
      status: response.status,
      statusText: response.ok ? 'OK' : 'Error',
      text: async () => response.body,
    } as unknown as Response
  }) as unknown as typeof fetch
}

describe('api()', () => {
  beforeEach(() => {
    globalThis.fetch = originalFetch
  })
  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  it('parses JSON responses on success', async () => {
    mockFetch({ ok: true, status: 200, body: JSON.stringify({ hello: 'world' }) })
    const result = await api<{ hello: string }>('/api/ping')
    expect(result.hello).toBe('world')
  })

  it('surfaces server error messages verbatim', async () => {
    mockFetch({ ok: false, status: 403, body: JSON.stringify({ error: 'forbidden' }) })
    await expect(api('/api/secret')).rejects.toThrow('forbidden')
  })

  it('falls back to HTTP status text when body is empty', async () => {
    mockFetch({ ok: false, status: 500, body: '' })
    await expect(api('/api/boom')).rejects.toThrow('Error')
  })

  it('wraps non-JSON error bodies so the caller still gets a message', async () => {
    mockFetch({ ok: false, status: 502, body: 'upstream unreachable' })
    await expect(api('/api/down')).rejects.toThrow('upstream unreachable')
  })

  it('serializes the json option as a JSON body with Content-Type', async () => {
    const spy = vi.fn<typeof fetch>(async () => ({
      ok: true,
      status: 200,
      statusText: 'OK',
      text: async () => '{}',
    } as unknown as Response))
    globalThis.fetch = spy as unknown as typeof fetch
    await api('/api/echo', { method: 'POST', json: { foo: 'bar' } })
    expect(spy).toHaveBeenCalledOnce()
    const call = spy.mock.calls[0]
    const init = call?.[1]
    expect((init?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect((init?.headers as Record<string, string>)['X-PMS-Client']).toBe('web')
    expect(init?.body).toBe(JSON.stringify({ foo: 'bar' }))
  })

  it('disables the HTTP cache so refetches after writes see fresh data', async () => {
    const spy = vi.fn<typeof fetch>(async () => ({
      ok: true,
      status: 200,
      statusText: 'OK',
      text: async () => '{}',
    } as unknown as Response))
    globalThis.fetch = spy as unknown as typeof fetch
    await api('/api/properties/1/cleaning/logs?month=2026-04')
    const init = spy.mock.calls[0]?.[1]
    expect(init?.cache).toBe('no-store')
  })

  it('prefixes requests with VITE_API_BASE_URL when set (cross-origin deploy)', async () => {
    // Re-import the module with a custom base URL so the top-level
    // `resolveBase()` picks up our override.
    vi.resetModules()
    vi.stubEnv('VITE_API_BASE_URL', 'https://api.pms.airport.sk/')
    try {
      const spy = vi.fn<typeof fetch>(async () => ({
        ok: true,
        status: 200,
        statusText: 'OK',
        text: async () => '{}',
      } as unknown as Response))
      globalThis.fetch = spy as unknown as typeof fetch
      const mod = await import('./http')
      await mod.api('/api/auth/me')
      expect(spy).toHaveBeenCalledOnce()
      expect(spy.mock.calls[0]?.[0]).toBe('https://api.pms.airport.sk/api/auth/me')
      expect(spy.mock.calls[0]?.[1]?.credentials).toBe('include')
    } finally {
      vi.unstubAllEnvs()
      vi.resetModules()
    }
  })

  it('prefers window.__PMS_CONFIG__.apiBaseUrl over VITE_API_BASE_URL (runtime config)', async () => {
    vi.resetModules()
    vi.stubEnv('VITE_API_BASE_URL', 'https://build-time.example.com')
    const previous = window.__PMS_CONFIG__
    window.__PMS_CONFIG__ = { apiBaseUrl: 'https://runtime.example.com/' }
    try {
      const spy = vi.fn<typeof fetch>(async () => ({
        ok: true,
        status: 200,
        statusText: 'OK',
        text: async () => '{}',
      } as unknown as Response))
      globalThis.fetch = spy as unknown as typeof fetch
      const mod = await import('./http')
      await mod.api('/api/auth/me')
      expect(spy.mock.calls[0]?.[0]).toBe('https://runtime.example.com/api/auth/me')
    } finally {
      window.__PMS_CONFIG__ = previous
      vi.unstubAllEnvs()
      vi.resetModules()
    }
  })
})
