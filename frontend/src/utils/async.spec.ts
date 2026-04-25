import { describe, it, expect, vi } from 'vitest'
import { sleep } from './async'

describe('sleep', () => {
  it('resolves after the requested delay', async () => {
    vi.useFakeTimers()
    try {
      let resolved = false
      const p = sleep(100).then(() => {
        resolved = true
      })
      expect(resolved).toBe(false)
      await vi.advanceTimersByTimeAsync(100)
      await p
      expect(resolved).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})
