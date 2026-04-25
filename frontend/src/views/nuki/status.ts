export type NukiBadgeTone = 'success' | 'warning' | 'danger' | 'info' | 'neutral'

export function displayStatus(status?: string): string {
  switch (status) {
    case 'generated': return 'Generated'
    case 'revoked': return 'Revoked'
    case 'not_generated': return 'Not generated'
    case 'success': return 'Healthy'
    case 'failure': return 'Failed'
    case 'partial': return 'Partial'
    case 'running': return 'Running'
    case 'manual': return 'Manual'
    case 'after_generate_refresh': return 'Post-generate refresh'
    case 'generate_all': return 'Generate all'
    case 'generate_one': return 'Generate one'
    default: return status ? status.replaceAll('_', ' ') : 'Unknown'
  }
}

export function statusTone(status?: string): NukiBadgeTone {
  if (status === 'generated' || status === 'success') return 'success'
  if (status === 'revoked' || status === 'partial' || status === 'running') return 'warning'
  if (status === 'failure') return 'danger'
  if (status === 'not_generated' || !status) return 'neutral'
  return 'neutral'
}

export function canGenerate(status?: string): boolean {
  return status === 'not_generated' || status === 'revoked' || !status
}
