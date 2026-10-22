import type { InvoiceItem } from '@/api/types/invoice'

export interface InvoiceItemForm {
  description: string
  quantity: string
  unit_price_eur: string
}

const maxCents = 1_000_000_000_000

export function accommodationItem(language: string, cents = 0): InvoiceItemForm {
  return {
    description: language === 'sk' ? 'Ubytovanie' : 'Accommodation',
    quantity: '1',
    unit_price_eur: (cents / 100).toFixed(2),
  }
}

export function invoiceItemValue(item: InvoiceItemForm): InvoiceItem {
  const quantity = String(item.quantity).trim()
  const price = String(item.unit_price_eur).trim()
  if (!/^[0-9]{1,7}(\.[0-9]{1,6})?$/.test(quantity)) {
    throw new Error('Quantity must be a positive decimal with up to 6 decimal places.')
  }
  const [whole = '0', fraction = ''] = quantity.split('.')
  const scaled = BigInt(whole) * 1_000_000n + BigInt(fraction.padEnd(6, '0'))
  if (scaled <= 0n || scaled > 1_000_000_000_000n) {
    throw new Error('Quantity must be greater than zero and at most 1,000,000.')
  }
  if (!/^[0-9]{1,11}(\.[0-9]{1,2})?$/.test(price)) {
    throw new Error('Unit price must be a nonnegative EUR amount with up to 2 decimal places.')
  }
  const [euros = '0', cents = ''] = price.split('.')
  const unitPrice = Number(euros) * 100 + Number(cents.padEnd(2, '0'))
  const lineTotal = (scaled * BigInt(unitPrice) + 500_000n) / 1_000_000n
  if (unitPrice > maxCents || lineTotal > BigInt(maxCents)) {
    throw new Error('Item amount exceeds the maximum supported amount.')
  }
  return {
    description: item.description.trim(),
    quantity,
    unit_price_cents: unitPrice,
    line_total_cents: Number(lineTotal),
  }
}

export function invoiceItemsPayload(rows: InvoiceItemForm[]): InvoiceItem[] {
  if (rows.length < 1 || rows.length > 100) throw new Error('Add between 1 and 100 items.')
  const items = rows.map((row, index) => {
    const item = invoiceItemValue(row)
    if (!item.description || Array.from(item.description).length > 500) {
      throw new Error(`Item ${index + 1} description must contain 1 to 500 characters.`)
    }
    return item
  })
  if (items.reduce((sum, item) => sum + item.line_total_cents, 0) > maxCents) {
    throw new Error('Invoice total exceeds the maximum supported amount.')
  }
  return items
}
