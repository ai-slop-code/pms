import { describe, it, expect } from 'vitest'
import {
  EM_DASH,
  formatEmpty,
  formatEuros,
  formatPercent,
  formatShortDate,
  formatShortDateTime,
  formatYesNo,
  isoTitle,
} from './format'

describe('formatEmpty', () => {
  it('returns em-dash for null and undefined', () => {
    expect(formatEmpty(null)).toBe(EM_DASH)
    expect(formatEmpty(undefined)).toBe(EM_DASH)
  })
  it('treats whitespace-only strings as empty', () => {
    expect(formatEmpty('   ')).toBe(EM_DASH)
  })
  it('returns the trimmed value when non-empty', () => {
    expect(formatEmpty('  hello ')).toBe('hello')
  })
  it('handles numbers', () => {
    expect(formatEmpty(0)).toBe('0')
    expect(formatEmpty(Number.NaN)).toBe(EM_DASH)
  })
  it('supports a custom fallback', () => {
    expect(formatEmpty(null, 'none')).toBe('none')
  })
})

describe('formatYesNo', () => {
  it('renders true/false as Yes/No', () => {
    expect(formatYesNo(true)).toBe('Yes')
    expect(formatYesNo(false)).toBe('No')
  })
  it('falls back for null/undefined', () => {
    expect(formatYesNo(null)).toBe(EM_DASH)
    expect(formatYesNo(undefined, 'n/a')).toBe('n/a')
  })
})

describe('formatShortDate / formatShortDateTime', () => {
  const iso = '2026-04-03T08:30:00Z'

  it('renders a month + day + year for valid ISO strings', () => {
    const out = formatShortDate(iso)
    expect(out).toMatch(/\d/)
    expect(out).toContain('2026')
  })

  it('falls back on empty or invalid input', () => {
    expect(formatShortDate(null)).toBe(EM_DASH)
    expect(formatShortDate('not a date')).toBe(EM_DASH)
    expect(formatShortDateTime('')).toBe(EM_DASH)
  })

  it('includes a time portion for formatShortDateTime', () => {
    const out = formatShortDateTime(iso)
    expect(out).toContain('·')
    expect(out).toMatch(/\d{2}:\d{2}/)
  })
})

describe('isoTitle', () => {
  it('returns the original ISO string when provided', () => {
    expect(isoTitle('2026-04-03T08:30:00Z')).toBe('2026-04-03T08:30:00Z')
  })
  it('returns an ISO string for Date input', () => {
    const d = new Date('2026-01-15T12:00:00Z')
    expect(isoTitle(d)).toBe(d.toISOString())
  })
  it('returns undefined for empty/invalid', () => {
    expect(isoTitle(null)).toBeUndefined()
    expect(isoTitle('nope')).toBeUndefined()
  })
})

describe('formatEuros', () => {
  it('formats positive and negative cent amounts', () => {
    expect(formatEuros(12345)).toMatch(/123[.,]45/)
    // The locale formatter may place the currency symbol between the minus
    // sign and the digits (e.g. "-€5.00" in en-US), so only assert the pieces.
    expect(formatEuros(-500)).toMatch(/-/)
    expect(formatEuros(-500)).toMatch(/5[.,]00/)
  })
  it('formats zero', () => {
    expect(formatEuros(0)).toMatch(/0[.,]00/)
  })
  it('adds a leading + when signed option is set and value positive', () => {
    expect(formatEuros(1000, { signed: true })).toMatch(/^\+/)
    // Negative is already signed by the locale formatter.
    expect(formatEuros(-1000, { signed: true })).not.toMatch(/^\+/)
    // Zero should not be prefixed with +.
    expect(formatEuros(0, { signed: true })).not.toMatch(/^\+/)
  })
  it('returns em-dash for null / undefined / non-finite', () => {
    expect(formatEuros(null)).toBe(EM_DASH)
    expect(formatEuros(undefined)).toBe(EM_DASH)
    expect(formatEuros(Number.NaN)).toBe(EM_DASH)
    expect(formatEuros(Number.POSITIVE_INFINITY)).toBe(EM_DASH)
  })
})

describe('formatPercent', () => {
  it('rounds ratios to whole percents by default', () => {
    expect(formatPercent(0.642)).toBe('64%')
    expect(formatPercent(1)).toBe('100%')
    expect(formatPercent(0)).toBe('0%')
  })
  it('respects digits option', () => {
    expect(formatPercent(0.6425, { digits: 1 })).toBe('64.3%')
    expect(formatPercent(0.33333, { digits: 2 })).toBe('33.33%')
  })
  it('returns em-dash for null / undefined / non-finite', () => {
    expect(formatPercent(null)).toBe(EM_DASH)
    expect(formatPercent(undefined)).toBe(EM_DASH)
    expect(formatPercent(Number.NaN)).toBe(EM_DASH)
  })
})

describe('formatEmpty (non-finite numbers)', () => {
  it('returns em-dash for Infinity values', () => {
    expect(formatEmpty(Number.POSITIVE_INFINITY)).toBe(EM_DASH)
    expect(formatEmpty(Number.NEGATIVE_INFINITY)).toBe(EM_DASH)
  })
})
