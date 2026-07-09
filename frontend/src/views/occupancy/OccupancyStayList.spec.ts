import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OccupancyStayList from './OccupancyStayList.vue'
import type { Occupancy } from '@/api/types/occupancy'

function occupancy(overrides: Partial<Occupancy> = {}): Occupancy {
  return {
    id: 1,
    source_type: 'booking_ics',
    source_event_uid: 'uid-1',
    start_at: '2026-08-01T00:00:00Z',
    end_at: '2026-08-03T00:00:00Z',
    status: 'active',
    raw_summary: 'Booking stay',
    last_synced_at: '2026-07-01T00:00:00Z',
    cleaning_calendar_excluded: false,
    ...overrides,
  }
}

describe('OccupancyStayList outcomes', () => {
  it('renders outcome badges and clear action separately from closure labels', () => {
    const w = mount(OccupancyStayList, {
      props: {
        month: '2026-08',
        statusFilter: '',
        occupancies: [occupancy({ stay_outcome: 'no_show' })],
      },
    })

    expect(w.text()).toContain('No-show')
    expect(w.text()).toContain('Clear outcome')
    expect(w.text()).not.toContain('Externally sold')
  })

  it('emits mark outcome actions for eligible Booking.com stays', async () => {
    const row = occupancy()
    const w = mount(OccupancyStayList, {
      props: {
        month: '2026-08',
        statusFilter: '',
        occupancies: [row],
      },
    })

    const button = w.findAll('button').find((b) => b.text() === 'Mark no-show')
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(w.emitted('markOutcome')?.[0]).toEqual([row, 'no_show'])
  })

  it('renders default cleaning-lady state and emits exclude action', async () => {
    const row = occupancy()
    const w = mount(OccupancyStayList, {
      props: {
        month: '2026-08',
        statusFilter: '',
        occupancies: [row],
      },
    })

    expect(w.text()).toContain('Cleaning lady: Yes')
    const button = w.findAll('button').find((b) => b.text() === 'Do not send cleaning event')
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(w.emitted('excludeCleaningCalendar')?.[0]).toEqual([row])
  })

  it('renders excluded cleaning-lady state and emits include action', async () => {
    const row = occupancy({
      cleaning_calendar_excluded: true,
      cleaning_calendar_exclusion_reason: 'Cleaner unavailable',
    })
    const w = mount(OccupancyStayList, {
      props: {
        month: '2026-08',
        statusFilter: '',
        occupancies: [row],
      },
    })

    expect(w.text()).toContain('Cleaning lady: No')
    expect(w.text()).toContain('Cleaner unavailable')
    const button = w.findAll('button').find((b) => b.text() === 'Mark as cleaned by cleaning lady')
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(w.emitted('includeCleaningCalendar')?.[0]).toEqual([row])
  })

  it('allows external-sale rows and disables stay-outcome rows for new exclusions', () => {
    const w = mount(OccupancyStayList, {
      props: {
        month: '2026-08',
        statusFilter: '',
        occupancies: [
          occupancy({ id: 1, closure_state: 'external_sale' }),
          occupancy({ id: 2, stay_outcome: 'no_show' }),
        ],
      },
    })

    expect(w.findAll('button').filter((b) => b.text() === 'Do not send cleaning event')).toHaveLength(1)
  })
})
