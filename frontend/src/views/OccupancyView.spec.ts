import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

import { api } from '@/api/http'
import OccupancyView from './OccupancyView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

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

function seedProperty(id = 6) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment F',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/occupancy-calendar')) {
      return Promise.resolve({
        calendar: {
          property_id: 6,
          month: '2026-07',
          raw_blocks: [],
          named_stays: [],
          availability_blocks: [],
        },
      })
    }
    if (url.includes('/occupancies')) return Promise.resolve({ occupancies: [] })
    if (url.includes('/occupancy-sync/runs')) return Promise.resolve({ runs: [] })
    if (url.includes('/occupancy-source')) {
      return Promise.resolve({ source: { active: false, source_type: '' } })
    }
    return Promise.resolve({})
  })
}

describe('OccupancyView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(OccupancyView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads the calendar tab on mount for the active property', async () => {
    seedProperty()
    apiRouter({})
    mount(OccupancyView)
    await flushPromises()
    const occCall = apiMock.mock.calls.find(
      ([u]) => typeof u === 'string' && u.includes('/occupancy-calendar'),
    )
    expect(occCall).toBeTruthy()
  })

  it('surfaces an error banner when the initial calendar load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('calendar 500'))
    const w = mount(OccupancyView)
    await flushPromises()
    expect(w.text()).toContain('calendar 500')
  })

  it('does not call deprecated export-token APIs from the sync tab', async () => {
    seedProperty()
    apiRouter({})
    const w = mount(OccupancyView)
    await flushPromises()

    await w.findAll('[role="tab"]')[2]?.trigger('click')
    await flushPromises()

    expect(apiMock.mock.calls.some(([u]) => typeof u === 'string' && u.includes('/occupancy-api-tokens'))).toBe(false)
    expect(w.text()).not.toContain('JSON export')
    expect(w.text()).not.toContain('n8n')
  })

  it('hides raw promotion and create actions when the selected night already has a named stay', async () => {
    seedProperty()
    apiRouter({})
    const w = mount(OccupancyView)
    await flushPromises()

    const rawBlock = {
      id: 10,
      property_id: 6,
      source_type: 'booking_ics',
      source_event_uid: 'raw-10',
      check_in_date: '2026-07-19',
      check_out_date: '2026-07-24',
      status: 'active',
      covered_nights: ['2026-07-20'],
      cleaning_events: [],
    }
    const namedStay = {
      id: 20,
      property_id: 6,
      display_name: 'Pipik1',
      stay_type: 'booking_com',
      check_in_date: '2026-07-20',
      check_out_date: '2026-07-24',
      status: 'active',
      cleaning_required: true,
      review_status: 'confirmed',
      counts_as_sold: true,
      has_finance_evidence: true,
      nuki_generation_status: 'generated',
      covered_nights: ['2026-07-20'],
      source_links: [
        {
          id: 30,
          source_type: 'booking_ics',
          source_event_uid: 'raw-10',
          linked_check_in_date: '2026-07-20',
          linked_check_out_date: '2026-07-24',
          link_status: 'source_deleted',
          conflict_reason: 'raw_source_missing',
        },
      ],
      cleaning_events: [],
    }
    const vm = w.vm as unknown as Record<string, unknown>
    ;(vm.onCalendarV2CellClick as (payload: Record<string, unknown>) => void)({
      dateKey: '2026-07-20',
      rawBlocks: [rawBlock],
      namedStays: [namedStay],
      availabilityBlocks: [],
    })
    await flushPromises()

    const dialog = document.body.querySelector<HTMLElement>('[aria-label="Calendar details for 2026-07-20"]')
    expect(dialog?.textContent).toContain('Pipik1')
    expect(dialog?.textContent).toContain('Finance confirmed')
    expect(dialog?.textContent).toContain('payout or statement data confirms this stay')
    expect(dialog?.textContent).not.toContain('Raw source issue')
    expect(dialog?.textContent).not.toContain('Raw Booking.com blocks')
    expect(dialog?.textContent).not.toContain('Promote to stay')
    expect(dialog?.textContent).not.toContain('Create stay')
    expect(dialog?.textContent).not.toContain('Block availability')
    w.unmount()
  })

  it('keeps raw promotion and create actions on an unassigned leftover night', async () => {
    seedProperty()
    apiRouter({})
    const w = mount(OccupancyView)
    await flushPromises()

    const vm = w.vm as unknown as Record<string, unknown>
    ;(vm.onCalendarV2CellClick as (payload: Record<string, unknown>) => void)({
      dateKey: '2026-07-19',
      rawBlocks: [
        {
          id: 10,
          property_id: 6,
          source_type: 'booking_ics',
          source_event_uid: 'raw-10',
          check_in_date: '2026-07-19',
          check_out_date: '2026-07-24',
          status: 'active',
          covered_nights: ['2026-07-19'],
          cleaning_events: [],
        },
      ],
      namedStays: [],
      availabilityBlocks: [],
    })
    await flushPromises()

    const dialog = document.body.querySelector<HTMLElement>('[aria-label="Calendar details for 2026-07-19"]')
    expect(dialog?.textContent).toContain('Raw Booking.com blocks')
    expect(dialog?.textContent).toContain('Promote to stay')
    expect(dialog?.textContent).toContain('Create stay')
    expect(dialog?.textContent).toContain('Block availability')
    w.unmount()
  })

  it.each([
    ['maintenance', false],
    ['personal_use', false],
    ['external', true],
    ['booking_com', true],
  ])('sends the backend cleaning default for %s manual stays', async (stayType, expectedCleaning) => {
    seedProperty()
    let payload: Record<string, unknown> | undefined
    apiMock.mockImplementation((url: string, options?: { json?: Record<string, unknown> }) => {
      if (url.endsWith('/stays')) {
        payload = options?.json
        return Promise.resolve({ ok: true })
      }
      if (url.includes('/occupancy-calendar')) {
        return Promise.resolve({
          calendar: { property_id: 6, month: '2026-07', raw_blocks: [], named_stays: [], availability_blocks: [] },
        })
      }
      return Promise.resolve({})
    })
    const w = mount(OccupancyView)
    await flushPromises()

    const vm = w.vm as unknown as Record<string, unknown>
    ;(vm.openManualStayDialog as (dateKey: string) => void)('2026-07-10')
    vm.manualStayDisplayName = 'Manual stay'
    vm.manualStayType = stayType
    await flushPromises()
    await (vm.submitManualStay as () => Promise<void>)()
    await flushPromises()

    expect(payload?.stay_type).toBe(stayType)
    expect(payload?.cleaning_required).toBe(expectedCleaning)
  })

  it('keeps explicit manual cleaning override when stay type changes', async () => {
    seedProperty()
    let payload: Record<string, unknown> | undefined
    apiMock.mockImplementation((url: string, options?: { json?: Record<string, unknown> }) => {
      if (url.endsWith('/stays')) {
        payload = options?.json
        return Promise.resolve({ ok: true })
      }
      if (url.includes('/occupancy-calendar')) {
        return Promise.resolve({
          calendar: { property_id: 6, month: '2026-07', raw_blocks: [], named_stays: [], availability_blocks: [] },
        })
      }
      return Promise.resolve({})
    })
    const w = mount(OccupancyView)
    await flushPromises()

    const vm = w.vm as unknown as Record<string, unknown>
    ;(vm.openManualStayDialog as (dateKey: string) => void)('2026-07-10')
    vm.manualStayDisplayName = 'Manual stay'
    vm.manualStayType = 'maintenance'
    await flushPromises()
    vm.manualStayCleaningRequired = true
    vm.manualStayCleaningManuallyChanged = true
    vm.manualStayType = 'personal_use'
    await flushPromises()
    await (vm.submitManualStay as () => Promise<void>)()
    await flushPromises()

    expect(payload?.stay_type).toBe('personal_use')
    expect(payload?.cleaning_required).toBe(true)
  })

  it('edits a named stay through the PMS 21 patch endpoint', async () => {
    seedProperty()
    let patchURL = ''
    let payload: Record<string, unknown> | undefined
    apiMock.mockImplementation((url: string, options?: { method?: string; json?: Record<string, unknown> }) => {
      if (url.includes('/stays/42') && options?.method === 'PATCH') {
        patchURL = url
        payload = options.json
        return Promise.resolve({ ok: true })
      }
      if (url.includes('/occupancy-calendar')) {
        return Promise.resolve({
          calendar: { property_id: 6, month: '2026-07', raw_blocks: [], named_stays: [], availability_blocks: [] },
        })
      }
      return Promise.resolve({})
    })
    const w = mount(OccupancyView)
    await flushPromises()

    const vm = w.vm as unknown as Record<string, unknown>
    ;(vm.openEditStayDialog as (stay: Record<string, unknown>) => void)({
      id: 42,
      property_id: 6,
      display_name: 'Old stay',
      stay_type: 'external',
      check_in_date: '2026-07-10',
      check_out_date: '2026-07-12',
      status: 'active',
      cleaning_required: true,
      review_status: 'confirmed',
      nuki_generation_status: 'generated',
      covered_nights: [],
      source_links: [],
      cleaning_events: [],
    })
    vm.editStayDisplayName = 'Updated stay'
    vm.editStayType = 'maintenance'
    vm.editStayCheckIn = '2026-07-11'
    vm.editStayCheckOut = '2026-07-13'
    vm.editStayCleaningRequired = false
    await (vm.submitEditStay as () => Promise<void>)()
    await flushPromises()

    expect(patchURL).toContain('/api/properties/6/stays/42')
    expect(payload).toEqual({
      display_name: 'Updated stay',
      check_in: '2026-07-11',
      check_out: '2026-07-13',
      stay_type: 'maintenance',
      cleaning_required: false,
    })
  })

  it.each(['cancelled', 'archived', 'active'] as const)(
    'updates named stay status to %s through the PMS 21 status endpoint',
    async (status) => {
      seedProperty()
      let patchURL = ''
      let payload: Record<string, unknown> | undefined
      apiMock.mockImplementation((url: string, options?: { method?: string; json?: Record<string, unknown> }) => {
        if (url.includes('/stays/42/status') && options?.method === 'PATCH') {
          patchURL = url
          payload = options.json
          return Promise.resolve({ ok: true })
        }
        if (url.includes('/occupancy-calendar')) {
          return Promise.resolve({
            calendar: { property_id: 6, month: '2026-07', raw_blocks: [], named_stays: [], availability_blocks: [] },
          })
        }
        return Promise.resolve({})
      })
      const w = mount(OccupancyView)
      await flushPromises()

      const vm = w.vm as unknown as Record<string, unknown>
      await (vm.updateNamedStayStatus as (stay: Record<string, unknown>, status: string) => Promise<void>)({ id: 42 }, status)
      await flushPromises()

      expect(patchURL).toContain('/api/properties/6/stays/42/status')
      expect(payload).toEqual({ status })
    },
  )
})
