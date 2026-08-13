export type OccupancyBadgeTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

export function displayStatus(status?: string): string {
  switch (status) {
    case 'active':
      return 'Active'
    case 'updated':
      return 'Updated'
    case 'cancelled':
      return 'Cancelled'
    case 'deleted_from_source':
      return 'Deleted from source'
    case 'success':
      return 'Healthy'
    case 'failure':
      return 'Failed'
    case 'partial':
      return 'Partial'
    case 'partial_no_mutation':
      return 'Partial (no changes applied)'
    case 'running':
      return 'Running'
    case 'manual':
      return 'Manual'
    case 'scheduled':
      return 'Scheduled'
    default:
      return status ? status.replaceAll('_', ' ') : 'Unknown'
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
