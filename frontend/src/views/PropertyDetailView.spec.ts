import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: '9' } }),
  RouterLink: { template: '<a><slot /></a>' },
}))

import { api } from '@/api/http'
import PropertyDetailView from './PropertyDetailView.vue'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

const sampleProperty = {
  id: 9,
  name: 'Apartment Bratislava',
  timezone: 'Europe/Bratislava',
  default_language: 'sk',
  invoice_code: 'BA-01',
  address_line1: 'Main 1',
  city: 'Bratislava',
  postal_code: '81101',
  country: 'Slovakia',
  week_starts_on: 'monday',
}
const sampleSettings = {
  profile: {
    legal_owner_name: 'Owner s.r.o.',
    billing_name: 'Owner',
    contact_phone: '+421 900 000 000',
    default_check_in_time: '15:00',
    default_check_out_time: '11:00',
    cleaner_nuki_auth_id: '1234',
    parking_instructions: 'Street parking',
    wifi_ssid: 'APT-WIFI',
  },
  integrations: {},
}

/** Dispatch API calls by URL + method, matching the first key that prefixes `url` and matches `method`. */
function apiRouter(
  handlers: Array<{ url: string; method?: string; respond: () => unknown }>,
) {
  apiMock.mockImplementation((url: string, opts?: { method?: string }) => {
    const method = opts?.method ?? 'GET'
    const handler = handlers.find(
      (h) => url.startsWith(h.url) && (h.method ?? 'GET') === method,
    )
    if (!handler) throw new Error(`unexpected api call: ${method} ${url}`)
    return Promise.resolve(handler.respond())
  })
}

describe('PropertyDetailView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('loads the property and settings on mount and renders the header', async () => {
    apiRouter([
      { url: '/api/properties/9/settings', respond: () => sampleSettings },
      { url: '/api/properties/9', respond: () => ({ property: sampleProperty }) },
    ])
    const w = mount(PropertyDetailView)
    await flushPromises()
    expect(w.text()).toContain('Apartment Bratislava')
    expect(apiMock).toHaveBeenCalledWith('/api/properties/9')
    expect(apiMock).toHaveBeenCalledWith('/api/properties/9/settings')
  })

  it('surfaces an error banner when the property fetch fails', async () => {
    apiMock.mockRejectedValue(new Error('forbidden'))
    const w = mount(PropertyDetailView)
    await flushPromises()
    expect(w.text()).toContain('forbidden')
  })

  it('PATCHes the general details and shows a success banner', async () => {
    apiRouter([
      { url: '/api/properties/9/settings', respond: () => sampleSettings },
      { url: '/api/properties/9', method: 'PATCH', respond: () => ({}) },
      { url: '/api/properties/9', respond: () => ({ property: sampleProperty }) },
    ])
    const w = mount(PropertyDetailView)
    await flushPromises()
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    const patchCall = apiMock.mock.calls.find(
      ([u, o]) => u === '/api/properties/9' && (o as { method?: string } | undefined)?.method === 'PATCH',
    )
    expect(patchCall).toBeTruthy()
    expect(w.text()).toContain('General details saved.')
  })
})
