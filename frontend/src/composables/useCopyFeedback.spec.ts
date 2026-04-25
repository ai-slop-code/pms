import { describe, it, expect, vi, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { useCopyFeedback } from './useCopyFeedback'

afterEach(() => {
  vi.useRealTimers()
})

describe('useCopyFeedback', () => {
  it('sets the copied key on flash and clears after the duration', async () => {
    vi.useFakeTimers()
    const { copied, flash } = useCopyFeedback(500)
    expect(copied.value).toBeNull()

    flash('pin-1')
    expect(copied.value).toBe('pin-1')

    vi.advanceTimersByTime(499)
    expect(copied.value).toBe('pin-1')

    vi.advanceTimersByTime(1)
    await nextTick()
    expect(copied.value).toBeNull()
  })

  it('resets the timer when flash is called again before expiry', () => {
    vi.useFakeTimers()
    const { copied, flash } = useCopyFeedback(500)
    flash('a')
    vi.advanceTimersByTime(300)
    flash('b')
    expect(copied.value).toBe('b')
    vi.advanceTimersByTime(300)
    // Original 500 ms window from the first flash would have expired,
    // but the second flash should have reset the timer.
    expect(copied.value).toBe('b')
    vi.advanceTimersByTime(200)
    expect(copied.value).toBeNull()
  })
})
