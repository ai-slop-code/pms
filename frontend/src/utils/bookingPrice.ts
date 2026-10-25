const maxSafeCents = BigInt(Number.MAX_SAFE_INTEGER)

export function parseBookingAdvertisedNightlyPrice(value: string): number | null {
  const trimmed = value.trim()
  if (trimmed === '') return null
  if (!/^\d+(?:\.\d{1,2})?$/.test(trimmed)) {
    throw new Error('Enter a price greater than zero with up to 2 decimal places, or leave it blank.')
  }
  const [whole = '0', fraction = ''] = trimmed.split('.')
  const cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0'))
  if (cents <= 0n || cents > maxSafeCents) {
    throw new Error('Enter a price greater than zero with up to 2 decimal places, or leave it blank.')
  }
  return Number(cents)
}

export function formatBookingAdvertisedNightlyPrice(cents: number | null | undefined): string {
  return cents === null || cents === undefined ? '' : (cents / 100).toFixed(2)
}
