import { describe, expect, it } from 'vitest'
import { activeNights, displayStatus, statusTone } from './status'

describe('occupancy status helpers (PMS_19)', () => {
  it('labels partial_no_mutation clearly', () => {
    expect(displayStatus('partial_no_mutation')).toBe('Partial (no changes applied)')
    expect(statusTone('partial_no_mutation')).toBe('warning')
  })

  it('activeNights prefers covered_nights night-level truth', () => {
    // An aggregate July 9-12 block whose July 11 was claimed by a named stay
    // reports covered_nights = [9,10] so July 11 is not double counted.
    const aggregate = {
      start_at: '2026-07-09T00:00:00Z',
      end_at: '2026-07-12T00:00:00Z',
      covered_nights: ['2026-07-09', '2026-07-10'],
    }
    const nights = activeNights(aggregate)
    expect(nights.has('2026-07-09')).toBe(true)
    expect(nights.has('2026-07-10')).toBe(true)
    expect(nights.has('2026-07-11')).toBe(false)
  })

  it('falls back to start/end span when covered_nights missing', () => {
    const nights = activeNights({ start_at: '2026-07-09T00:00:00Z', end_at: '2026-07-11T00:00:00Z' })
    expect(nights.has('2026-07-09')).toBe(true)
    expect(nights.has('2026-07-10')).toBe(true)
    expect(nights.has('2026-07-11')).toBe(false)
  })
})
