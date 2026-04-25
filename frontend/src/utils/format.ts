/**
 * Display helpers enforcing the UI copy rules from PMS_07/PMS_08:
 *  - empty values render as an em-dash
 *  - dates render as short locale strings, with the raw ISO exposed via title=""
 *  - booleans render as "Yes" / "No"
 */

export const EM_DASH = '—'

export function formatEmpty(
  value: string | number | null | undefined,
  fallback: string = EM_DASH,
): string {
  if (value === null || value === undefined) return fallback
  if (typeof value === 'number') {
    return Number.isFinite(value) ? String(value) : fallback
  }
  const trimmed = value.trim()
  return trimmed === '' ? fallback : trimmed
}

export function formatYesNo(value: boolean | null | undefined, fallback = EM_DASH): string {
  if (value === null || value === undefined) return fallback
  return value ? 'Yes' : 'No'
}

function parseDate(value: string | Date | null | undefined): Date | null {
  if (!value) return null
  const d = value instanceof Date ? value : new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

/** Short locale date, e.g. "3 Apr 2026". Returns fallback for empty/invalid input. */
export function formatShortDate(
  value: string | Date | null | undefined,
  fallback = EM_DASH,
): string {
  const d = parseDate(value)
  if (!d) return fallback
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}

/** Short locale date + hh:mm time, for timestamps. */
export function formatShortDateTime(
  value: string | Date | null | undefined,
  fallback = EM_DASH,
): string {
  const d = parseDate(value)
  if (!d) return fallback
  return `${formatShortDate(d)} · ${d.toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  })}`
}

/**
 * Returns the original ISO-8601 string to use inside a `title=""` attribute for
 * exact on-hover inspection. Returns `undefined` when the input is empty so the
 * consumer can omit the attribute.
 */
export function isoTitle(value: string | Date | null | undefined): string | undefined {
  const d = parseDate(value)
  if (!d) return undefined
  if (value instanceof Date) return d.toISOString()
  // Prefer the caller's raw string when it was already an ISO-looking timestamp.
  return typeof value === 'string' ? value : d.toISOString()
}

/**
 * Formats euro cents as a locale string with the € suffix. Displayed as
 * `"1 234,56 €"` in locales that prefer the trailing symbol; returns the
 * em-dash fallback when the input isn't a finite number.
 *
 * @param cents Integer amount in euro cents.
 * @param options.signed Force a leading `+` on non-negative values (useful
 *   for "delta vs previous month" columns in Finance and Analytics).
 */
export function formatEuros(
  cents: number | null | undefined,
  options: { signed?: boolean } = {},
): string {
  if (cents === null || cents === undefined || !Number.isFinite(cents)) return EM_DASH
  const euros = cents / 100
  const formatted = euros.toLocaleString(undefined, {
    style: 'currency',
    currency: 'EUR',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
  return options.signed && euros > 0 ? `+${formatted}` : formatted
}

/**
 * Formats a ratio in [0, 1] as a percentage string, e.g. `0.642 -> "64%"`.
 * Returns the em-dash fallback when the input isn't a finite number.
 */
export function formatPercent(
  ratio: number | null | undefined,
  options: { digits?: number } = {},
): string {
  if (ratio === null || ratio === undefined || !Number.isFinite(ratio)) return EM_DASH
  return `${(ratio * 100).toFixed(options.digits ?? 0)}%`
}

