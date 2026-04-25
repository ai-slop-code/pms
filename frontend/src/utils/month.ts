/**
 * Month-key helpers. Canonical format is `YYYY-MM` (matches the backend API
 * and the native `<input type="month">` value). Extracted from the copies
 * that used to live inside CleaningView, FinanceView, BookingPayoutsView,
 * and OccupancyView.
 *
 * All computations use the browser's local time — `YYYY-MM` in this app is
 * always "the user's civil calendar month" (e.g. the month they're about to
 * enter in a `<input type="month">`), never an absolute UTC slice. Using
 * UTC here would flip the key at midnight in negative-UTC timezones.
 */

const MONTH_RE = /^(\d{4})-(\d{2})$/

/**
 * Shift a `YYYY-MM` key by an integer number of months. Positive `delta`
 * moves into the future. Returns the current-month key when the input
 * doesn't match the canonical pattern — matches the defensive behaviour
 * every inline copy inherited from FinanceView.
 */
export function shiftMonth(key: string, delta: number): string {
  const m = MONTH_RE.exec(key)
  if (!m) return monthKey(new Date())
  const year = Number(m[1])
  const month = Number(m[2])
  if (!Number.isFinite(year) || !Number.isFinite(month) || month < 1 || month > 12) {
    return monthKey(new Date())
  }
  // JS Date handles month overflow/underflow for us.
  const d = new Date(year, month - 1 + delta, 1)
  return monthKey(d)
}

/** Current-month `YYYY-MM` key for the given date (defaults to now, local time). */
export function monthKey(date: Date = new Date()): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  return `${y}-${m}`
}

/**
 * Parse a `YYYY-MM` key into `{ year, month }` (month is 1-based). Falls back
 * to the current civil calendar month when the input is malformed.
 */
export function parseMonthKey(key: string): { year: number; month: number } {
  const m = MONTH_RE.exec(key)
  if (!m) {
    const now = new Date()
    return { year: now.getFullYear(), month: now.getMonth() + 1 }
  }
  return { year: Number(m[1]), month: Number(m[2]) }
}

