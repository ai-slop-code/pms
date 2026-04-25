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
  return 'neutral'
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
