import { describe, expect, it } from 'vitest'
import { invoiceItemValue, invoiceItemsPayload } from './items'

describe('invoice item arithmetic', () => {
  it.each([
    ['1.5', '1.01', 152],
    ['0.5', '0.01', 1],
    ['0.499999', '0.01', 0],
    ['0.000001', '9999999999.99', 1000000],
    ['1000000', '10000.00', 1000000000000],
    ['2.25', '123.99', 27898],
  ])('rounds %s × %s EUR to %s cents exactly', (quantity, unit_price_eur, expected) => {
    expect(invoiceItemValue({ description: 'Cleaning', quantity, unit_price_eur }).line_total_cents).toBe(expected)
  })

  it('rounds individual lines before summing', () => {
    const item = { description: 'Cleaning', quantity: '0.5', unit_price_eur: '0.01' }
    expect(invoiceItemsPayload([item, item]).reduce((total, row) => total + row.line_total_cents, 0)).toBe(2)
  })

  it.each(['', '0', '-1', '1e2', 'NaN', '1.0000001', '1000000.000001'])('rejects quantity %s', (quantity) => {
    expect(() => invoiceItemsPayload([{ description: 'Cleaning', quantity, unit_price_eur: '1.00' }])).toThrow()
  })

  it('rejects aggregate overflow and missing descriptions', () => {
    const item = { description: 'Cleaning', quantity: '1', unit_price_eur: '10000000000.00' }
    expect(() => invoiceItemsPayload([item, item])).toThrow('total exceeds')
    expect(() => invoiceItemsPayload([{ ...item, description: ' ' }])).toThrow('description')
  })
})
