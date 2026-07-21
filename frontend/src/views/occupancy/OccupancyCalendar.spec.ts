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
      counts_as_sold: true,
      has_finance_evidence: false,
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
  it('shows raw-only nights and renders a promoted multi-night stay as one connected ribbon', () => {
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar },
    })

    const rawOnlyCell = w
      .findAll('.calendar__cell')
      .find((cell) => cell.attributes('aria-label')?.startsWith('2026-07-09'))
    const stayStartCell = w
      .findAll('.calendar__cell')
      .find((cell) => cell.attributes('aria-label')?.startsWith('2026-07-10'))
    const stayEndCell = w
      .findAll('.calendar__cell')
      .find((cell) => cell.attributes('aria-label')?.startsWith('2026-07-11'))
    const bands = w.find('.calendar').findAll('.calendar__stay-band')

    expect(rawOnlyCell?.find('.calendar__chip--raw').exists()).toBe(true)
    expect(stayStartCell?.find('.calendar__chip--raw').exists()).toBe(false)
    expect(stayEndCell?.find('.calendar__chip--raw').exists()).toBe(false)
    expect(stayStartCell?.attributes('aria-label')).not.toContain('raw booking block')
    expect(bands).toHaveLength(1)
    expect(bands[0]!.text()).toContain('Named Guest · 2 nights')
    expect(bands[0]!.attributes('style')).toContain('grid-column: 5 / 7')
    expect(bands[0]!.classes()).toContain('calendar__stay-band--warning')
    expect(bands[0]!.classes()).toContain('calendar__stay-band--error')
    expect(bands[0]!.findAll('.calendar__stay-alert')).toHaveLength(1)
    expect(bands[0]!.attributes('title')).toContain('raw source issue')
    expect(bands[0]!.attributes('title')).toContain('Nuki error')
    expect(bands[0]!.attributes('title')).toContain('cleaning error')
    expect(w.text()).toContain('blocked')
    expect(w.text()).toContain('Connected ribbons represent one continuous stay')
  })

  it('splits a stay at the week boundary and marks both segments as continuations', () => {
    const crossWeekCalendar: OccupancyCalendarView = {
      ...calendar,
      raw_blocks: [],
      availability_blocks: [],
      named_stays: [
        {
          ...calendar.named_stays[0]!,
          id: 21,
          display_name: 'Weekend Guest',
          check_in_date: '2026-07-11',
          check_out_date: '2026-07-14',
          covered_nights: ['2026-07-11', '2026-07-12', '2026-07-13'],
          source_links: [],
          cleaning_events: [],
          nuki_generation_status: 'generated',
          nuki_generation_error: undefined,
        },
      ],
    }
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar: crossWeekCalendar },
    })
    const bands = w.find('.calendar').findAll('.calendar__stay-band')

    expect(bands).toHaveLength(2)
    expect(bands[0]!.attributes('style')).toContain('grid-column: 6 / 8')
    expect(bands[0]!.classes()).toContain('calendar__stay-band--continues-after')
    expect(bands[1]!.attributes('style')).toContain('grid-column: 1 / 2')
    expect(bands[1]!.classes()).toContain('calendar__stay-band--continues-before')
    expect(bands[0]!.text()).toContain('Weekend Guest · 3 nights')
    expect(bands[1]!.text()).toContain('Weekend Guest · 3 nights')
  })

  it('does not style missing ICS provenance as a warning when finance evidence confirms the stay', () => {
    const financeConfirmedCalendar: OccupancyCalendarView = {
      ...calendar,
      named_stays: [
        {
          ...calendar.named_stays[0]!,
          has_finance_evidence: true,
          nuki_generation_status: 'generated',
          nuki_generation_error: undefined,
          cleaning_events: [],
          source_links: [
            {
              ...calendar.named_stays[0]!.source_links[0]!,
              link_status: 'source_deleted',
              conflict_reason: 'raw_source_missing',
            },
          ],
        },
      ],
    }
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar: financeConfirmedCalendar },
    })
    const band = w.find('.calendar__stay-band')

    expect(band.classes()).not.toContain('calendar__stay-band--warning')
    expect(band.attributes('title')).not.toContain('raw source issue')
  })

  it('keeps incomplete raw coverage conflicts actionable despite finance evidence', () => {
    const conflictingCalendar: OccupancyCalendarView = {
      ...calendar,
      named_stays: [
        {
          ...calendar.named_stays[0]!,
          has_finance_evidence: true,
          nuki_generation_status: 'generated',
          nuki_generation_error: undefined,
          cleaning_events: [],
        },
      ],
    }
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar: conflictingCalendar },
    })
    const band = w.find('.calendar__stay-band')

    expect(band.classes()).toContain('calendar__stay-band--warning')
    expect(band.attributes('title')).toContain('raw source issue')
  })

  it('opens the existing day detail flow when a stay band is clicked', async () => {
    const w = mount(OccupancyCalendar, {
      props: { month: '2026-07', occupancies: [], calendar },
    })

    await w.find('.calendar__stay-band').trigger('click')

    expect(w.emitted('calendar-cell-click')?.[0]).toEqual([
      {
        dateKey: '2026-07-10',
        rawBlocks: calendar.raw_blocks,
        namedStays: calendar.named_stays,
        availabilityBlocks: [],
      },
    ])
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
