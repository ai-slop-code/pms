import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

const pushMock = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
}))

import { api } from '@/api/http'
import PropertyFormView from './PropertyFormView.vue'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

function mountView() {
  return mount(PropertyFormView)
}

describe('PropertyFormView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
    pushMock.mockReset()
  })

  it('renders the form with prefilled defaults', () => {
    const w = mountView()
    expect(w.text()).toContain('New property')
    const inputs = w.findAll('input')
    expect(inputs.length).toBeGreaterThanOrEqual(3)
    expect((inputs[1]?.element as HTMLInputElement).value).toBe('Europe/Bratislava')
    expect((inputs[2]?.element as HTMLInputElement).value).toBe('sk')
  })

  it('POSTs the new property and navigates to /properties on success', async () => {
    apiMock.mockImplementation((_url: string, opts?: { method?: string }) => {
      if (opts?.method === 'POST') return Promise.resolve({})
      return Promise.resolve({ properties: [] })
    })
    const w = mountView()
    await w.findAll('input')[0]?.setValue('Apartment X')
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(apiMock).toHaveBeenCalledWith(
      '/api/properties',
      expect.objectContaining({
        method: 'POST',
        json: { name: 'Apartment X', timezone: 'Europe/Bratislava', default_language: 'sk' },
      }),
    )
    expect(pushMock).toHaveBeenCalledWith('/properties')
  })

  it('shows an error banner when the API rejects', async () => {
    apiMock.mockRejectedValueOnce(new Error('Duplicate name'))
    const w = mountView()
    await w.findAll('input')[0]?.setValue('X')
    await w.find('form').trigger('submit.prevent')
    await flushPromises()
    expect(w.text()).toContain('Duplicate name')
    expect(pushMock).not.toHaveBeenCalled()
  })

  it('cancels back to /properties without calling the API', async () => {
    const w = mountView()
    const cancelBtn = w.findAll('button').find((b) => b.text().includes('Cancel'))!
    await cancelBtn.trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/properties')
    expect(apiMock).not.toHaveBeenCalled()
  })
})
