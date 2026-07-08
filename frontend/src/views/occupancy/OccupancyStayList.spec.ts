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
})
