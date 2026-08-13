import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OccupancySyncPanel from './OccupancySyncPanel.vue'
import type { OccupancySyncRun } from '@/api/types/occupancy'

const run: OccupancySyncRun = {
  id: 1,
  started_at: '2026-08-11T10:00:00Z',
  finished_at: '2026-08-11T10:00:01Z',
  status: 'success',
  events_seen: 8,
  trigger: 'manual',
  raw_blocks_inserted: 1,
  raw_blocks_updated: 2,
  raw_blocks_unchanged: 3,
  raw_blocks_deleted_from_source: 4,
  raw_block_conflicts: 5,
}

describe('OccupancySyncPanel', () => {
  it('labels and displays canonical raw-block sync counters', () => {
    const wrapper = mount(OccupancySyncPanel, {
      props: {
        source: { active: true, source_type: 'booking_ics' },
        runs: [run],
        syncing: false,
      },
    })

    expect(wrapper.findAll('th').map((header) => header.text())).toEqual([
      'Started',
      'Status',
      'Events seen',
      'Raw blocks inserted',
      'Raw blocks updated',
      'Raw blocks unchanged',
      'Raw blocks deleted from source',
      'Raw block conflicts',
      'Trigger',
      'Error',
    ])
    expect(wrapper.findAll('tbody td').slice(2, 8).map((cell) => cell.text())).toEqual([
      '8',
      '1',
      '2',
      '3',
      '4',
      '5',
    ])
    expect(wrapper.text()).not.toContain('Upserted')
  })
})
