import { describe, it, expect } from 'vitest'
import { shiftMonth, monthKey } from './month'

describe('monthKey', () => {
  it('formats a date as YYYY-MM using local time', () => {
    expect(monthKey(new Date(2026, 0, 15))).toBe('2026-01')
    expect(monthKey(new Date(2026, 11, 31))).toBe('2026-12')
  })
})

describe('shiftMonth', () => {
  it('shifts forward within the same year', () => {
    expect(shiftMonth('2026-01', 3)).toBe('2026-04')
  })
  it('rolls over into the next year', () => {
    expect(shiftMonth('2026-11', 3)).toBe('2027-02')
  })
  it('rolls back into the previous year', () => {
    expect(shiftMonth('2026-02', -3)).toBe('2025-11')
  })
  it('returns zero delta unchanged', () => {
    expect(shiftMonth('2026-06', 0)).toBe('2026-06')
  })
  it('falls back to the current month for a malformed input', () => {
    expect(shiftMonth('not a month', 1)).toBe(monthKey(new Date()))
  })
  it('falls back to the current month when the month number is out of range', () => {
    expect(shiftMonth('2026-13', 0)).toBe(monthKey(new Date()))
  })
})

