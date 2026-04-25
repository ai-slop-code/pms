import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { useTransientMessage } from './useTransientMessage'

describe('useTransientMessage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('exposes an empty message initially', () => {
    const { message } = useTransientMessage()
    expect(message.value).toBe('')
  })

  it('clears the message after the default delay', () => {
    const { message, show } = useTransientMessage()
    show('Saved')
    expect(message.value).toBe('Saved')
    vi.advanceTimersByTime(2999)
    expect(message.value).toBe('Saved')
    vi.advanceTimersByTime(1)
    expect(message.value).toBe('')
  })

  it('respects a custom duration', () => {
    const { message, show } = useTransientMessage(500)
    show('Hi')
    vi.advanceTimersByTime(500)
    expect(message.value).toBe('')
  })

  it('resets the timer when a new message arrives', () => {
    const { message, show } = useTransientMessage(1000)
    show('First')
    vi.advanceTimersByTime(800)
    show('Second')
    vi.advanceTimersByTime(800)
    expect(message.value).toBe('Second')
    vi.advanceTimersByTime(200)
    expect(message.value).toBe('')
  })

  it('clear() wipes the message and cancels any pending timer', () => {
    const { message, show, clear } = useTransientMessage()
    show('Saved')
    clear()
    expect(message.value).toBe('')
    vi.advanceTimersByTime(5000)
    expect(message.value).toBe('')
  })

  it('show("") clears without scheduling a timer', () => {
    const { message, show } = useTransientMessage()
    show('Saved')
    show('')
    expect(message.value).toBe('')
    vi.advanceTimersByTime(5000)
    expect(message.value).toBe('')
  })
})
