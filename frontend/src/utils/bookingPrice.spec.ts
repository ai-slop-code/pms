import { describe, expect, it } from 'vitest'
import { formatBookingAdvertisedNightlyPrice, parseBookingAdvertisedNightlyPrice } from './bookingPrice'

describe('booking advertised nightly price', () => {
  it.each([
    ['100', 10000],
    ['100.5', 10050],
    ['99.95', 9995],
    ['0.01', 1],
  ])('converts %s to exact cents', (input, expected) => {
    expect(parseBookingAdvertisedNightlyPrice(input)).toBe(expected)
  })

  it.each(['', '  ', '0', '0.00', '-1', '1.234', 'EUR 10'])('rejects %j or treats it as absent', (input) => {
    if (input.trim() === '') expect(parseBookingAdvertisedNightlyPrice(input)).toBeNull()
    else expect(() => parseBookingAdvertisedNightlyPrice(input)).toThrow()
  })

  it('formats saved cents for edit forms', () => {
    expect(formatBookingAdvertisedNightlyPrice(9995)).toBe('99.95')
    expect(formatBookingAdvertisedNightlyPrice(null)).toBe('')
  })
})
