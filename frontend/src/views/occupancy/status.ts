export type OccupancyBadgeTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

export function displayStatus(status?: string): string {
  switch (status) {
    case 'active': return 'Active'
    case 'updated': return 'Updated'
    case 'cancelled': return 'Cancelled'
    case 'deleted_from_source': return 'Deleted from source'
    case 'success': return 'Healthy'
    case 'failure': return 'Failed'
    case 'partial': return 'Partial'
    case 'partial_no_mutation': return 'Partial (no changes applied)'
    case 'running': return 'Running'
    case 'manual': return 'Manual'
    case 'scheduled': return 'Scheduled'
    default: return status ? status.replaceAll('_', ' ') : 'Unknown'
  }
}

export function statusTone(status?: string): OccupancyBadgeTone {
  if (!status) return 'neutral'
  if (['active', 'success'].includes(status)) return 'success'
  if (['failure', 'cancelled', 'deleted_from_source'].includes(status)) return 'danger'
  if (['partial', 'running', 'updated'].includes(status)) return 'warning'
  if (status === 'partial_no_mutation') return 'warning'
  return 'neutral'
}

/**
 * activeNights returns the property-local nights a row actually occupies.
 * PMS_19: prefer the server-computed covered_nights (night-level truth from
 * occupancy_nights) so a shrunk aggregate never double-counts a named night;
 * fall back to the raw start/end span for rows without coverage data.
 */
export function activeNights(o: { covered_nights?: string[]; start_at: string; end_at: string }): Set<string> {
  if (Array.isArray(o.covered_nights)) return new Set(o.covered_nights)
  return nightsBetween(o.start_at, o.end_at)
}

export function nightsBetween(startIso: string, endIso: string): Set<string> {
  const set = new Set<string>()
  const cur = new Date(startIso)
  const end = new Date(endIso)
  while (cur < end) {
    set.add(cur.toISOString().slice(0, 10))
    cur.setUTCDate(cur.getUTCDate() + 1)
  }
  return set
}

export function nightsCount(startIso: string, endIso: string): number {
  const start = new Date(startIso)
  const end = new Date(endIso)
  const ms = end.getTime() - start.getTime()
  return Math.max(1, Math.round(ms / (24 * 60 * 60 * 1000)))
}
