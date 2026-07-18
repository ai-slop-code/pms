import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OccupancyCalendar from './OccupancyCalendar.vue'
import type { OccupancyCalendarView } from '@/api/types/occupancy'

const calendar: OccupancyCalendarView = {
  property_id: 1,
  month: '2026-07',
  raw_blocks: [
    {
      id: 10,
      property_id: 1,
      source_type: 'booking_ics',
      source_event_uid: 'raw-1',
      check_in_date: '2026-07-09',
      check_out_date: '2026-07-12',
      status: 'active',
      raw_summary: 'Booking raw',
      covered_nights: ['2026-07-09', '2026-07-10', '2026-07-11'],
      cleaning_events: [
        {
          id: 1,
          checkout_date: '2026-07-10',
          cleaning_kind: 'provisional_block',
          title: 'Upratovanie',
          status: 'pending',
        },
      ],
    },
    {
      id: 11,
      property_id: 1,
      source_type: 'booking_ics',
      source_event_uid: 'raw-2',
      check_in_date: '2026-07-10',
      check_out_date: '2026-07-11',
      status: 'active',
      covered_nights: ['2026-07-10'],
      cleaning_events: [],
    },
  ],
  named_stays: [
    {
      id: 20,
      property_id: 1,
      display_name: 'Named Guest',
      stay_type: 'booking_com',
      check_in_date: '2026-07-10',
      check_out_date: '2026-07-12',
      status: 'active',
      cleaning_required: true,
      review_status: 'confirmed',
      nuki_generation_status: 'error',
      nuki_generation_error: 'credentials missing',
      covered_nights: ['2026-07-10', '2026-07-11'],
      legacy_occupancy_id: 200,
      source_links: [
        {
          id: 30,
          raw_booking_block_id: 10,
          source_type: 'booking_ics',
          source_event_uid: 'raw-1',
          linked_check_in_date: '2026-07-10',
          linked_check_out_date: '2026-07-12',
          link_status: 'conflict',
        },
      ],
      cleaning_events: [
        {
          id: 2,
          checkout_date: '2026-07-12',
          cleaning_kind: 'named_stay',
          title: 'Upratovanie: Named Guest',
          status: 'error',
        },
      ],
    },
  ],
  availability_blocks: [
    {
      id: 40,
      property_id: 1,
      block_type: 'closed',
      start_date: '2026-07-20',
      end_date: '2026-07-21',
      reason: 'Repair',
      status: 'active',
      covered_nights: ['2026-07-20'],
    },
  ],
}

describe('OccupancyCalendar Stage 5 combined model', () => {
  it('renders raw, named, availability, source, Nuki, and cleaning badges distinctly', () => {
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar },
    })

    expect(w.text()).toContain('raw ×2')
    expect(w.text()).toContain('Named Guest')
    expect(w.text()).toContain('blocked')
    expect(w.text()).toContain('Raw source issue')
    expect(w.text()).toContain('Nuki error')
    expect(w.text()).toContain('Cleaning error')
    expect(w.text()).toContain('Raw-only nights')
  })

  it('emits calendar-cell-click for empty nights so manual stays can be created', async () => {
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar },
    })
    const emptyCell = w
      .findAll('.calendar__cell')
      .find((cell) => cell.attributes('aria-label')?.startsWith('2026-07-01'))

    expect(emptyCell).toBeTruthy()
    expect(emptyCell!.attributes('role')).toBe('button')
    await emptyCell!.trigger('click')

    expect(w.emitted('calendar-cell-click')?.[0]).toEqual([
      { dateKey: '2026-07-01', rawBlocks: [], namedStays: [], availabilityBlocks: [] },
    ])
  })
})
