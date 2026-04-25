import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({
  api: vi.fn(),
}))

const pushMock = vi.fn()
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock }),
  RouterLink: { template: '<a><slot /></a>' },
}))

import { api } from '@/api/http'
import PropertiesView from './PropertiesView.vue'
import { usePropertyStore, type Property } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

function makeProperty(overrides: Partial<Property> = {}): Property {
  return {
    id: 1,
    name: 'Sample',
    timezone: 'Europe/Bratislava',
    default_language: 'en',
    owner_user_id: 1,
    active: true,
    ...overrides,
  }
}

function mountView() {
  return mount(PropertiesView)
}

describe('PropertiesView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    apiMock.mockReset()
    apiMock.mockResolvedValue({ properties: [] })
    pushMock.mockReset()
  })

  it('renders the page header and triggers a list fetch on mount', async () => {
    const w = mountView()
    expect(w.text()).toContain('Properties')
    expect(w.text()).toContain('New property')
    // fetchList is called from onMounted; allow the microtask to flush.
    await Promise.resolve()
    expect(apiMock).toHaveBeenCalledWith('/api/properties')
  })

  it('shows the empty state when the list is empty', () => {
    const w = mountView()
    expect(w.text()).toContain('No properties found yet.')
  })

  it('renders a row per property from the store', async () => {
    const store = usePropertyStore()
    store.list = [
      makeProperty({ id: 1, name: 'Alpha', timezone: 'Europe/Bratislava' }),
      makeProperty({ id: 2, name: 'Beta', timezone: 'Europe/Prague' }),
    ]
    const w = mountView()
    const rows = w.findAll('tbody tr')
    expect(rows.length).toBe(2)
    expect(rows[0]?.text()).toContain('Alpha')
    expect(rows[0]?.text()).toContain('Europe/Bratislava')
    expect(rows[1]?.text()).toContain('Beta')
  })

  it('swallows fetch errors so the view still renders', async () => {
    apiMock.mockRejectedValueOnce(new Error('network down'))
    const w = mountView()
    await Promise.resolve()
    await Promise.resolve()
    expect(w.text()).toContain('Properties')
  })

  it('navigates to the new-property route when the header action is clicked', async () => {
    const w = mountView()
    await w.find('button').trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/properties/new')
  })
})
