export type DashboardBadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info'

export function widgetTitle(status: string): string {
  switch (status) {
    case 'ok':
      return 'Healthy'
    case 'error':
      return 'Needs attention'
    case 'partial':
      return 'Partial'
    case 'running':
      return 'Running'
    case 'no_sync_yet':
      return 'Not run yet'
    case 'not_configured':
      return 'Not configured'
    default:
      return status
  }
}

export function displayStatus(status?: string): string {
  switch (status) {
    case 'active':
      return 'Active'
    case 'updated':
      return 'Updated'
    case 'generated':
      return 'Generated'
    case 'revoked':
      return 'Revoked'
    case 'not_generated':
      return 'Not generated'
    default:
      return status ? status.replaceAll('_', ' ') : ''
  }
}

export function statusTone(status?: string): DashboardBadgeTone {
  switch (status) {
    case 'ok':
    case 'generated':
    case 'active':
      return 'success'
    case 'running':
    case 'updated':
      return 'info'
    case 'partial':
      return 'warning'
    case 'error':
    case 'not_generated':
    case 'revoked':
      return 'danger'
    default:
      return 'neutral'
  }
}
