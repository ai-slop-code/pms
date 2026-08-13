import { describe, expect, it } from 'vitest'
import { displayStatus, statusTone } from './status'

describe('occupancy sync status helpers', () => {
  it('labels partial_no_mutation clearly', () => {
    expect(displayStatus('partial_no_mutation')).toBe('Partial (no changes applied)')
    expect(statusTone('partial_no_mutation')).toBe('warning')
  })
})
