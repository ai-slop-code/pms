import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useToast } from './useToast'

describe('useToast composable', () => {
  beforeEach(() => {
    const { toasts } = useToast()
    toasts.splice(0, toasts.length)
  })

  it('push adds a toast with defaults and returns id', () => {
    const { push, toasts } = useToast()
    const id = push({ message: 'hi' })
    expect(typeof id).toBe('number')
    expect(toasts).toHaveLength(1)
    expect(toasts[0]?.tone).toBe('info')
    expect(toasts[0]?.message).toBe('hi')
  })

  it('success helper sets tone=success and default title', () => {
    const t = useToast()
    t.success('All saved')
    expect(t.toasts[0]?.tone).toBe('success')
    expect(t.toasts[0]?.title).toBe('Success')
    expect(t.toasts[0]?.message).toBe('All saved')
  })

  it('error helper sets tone=danger', () => {
    const t = useToast()
    t.error('Boom')
    expect(t.toasts[0]?.tone).toBe('danger')
  })

  it('dismiss removes the toast', () => {
    const t = useToast()
    const id = t.push({ message: 'bye', timeout: 0 })
    expect(t.toasts).toHaveLength(1)
    t.dismiss(id)
    expect(t.toasts).toHaveLength(0)
  })

  it('auto-dismisses after the configured timeout', () => {
    vi.useFakeTimers()
    try {
      const t = useToast()
      t.push({ message: 'vanish', timeout: 1000 })
      expect(t.toasts).toHaveLength(1)
      vi.advanceTimersByTime(1001)
      expect(t.toasts).toHaveLength(0)
    } finally {
      vi.useRealTimers()
    }
  })

  it('timeout=0 never auto-dismisses', () => {
    vi.useFakeTimers()
    try {
      const t = useToast()
      t.push({ message: 'forever', timeout: 0 })
      vi.advanceTimersByTime(60_000)
      expect(t.toasts).toHaveLength(1)
    } finally {
      vi.useRealTimers()
    }
  })
})
