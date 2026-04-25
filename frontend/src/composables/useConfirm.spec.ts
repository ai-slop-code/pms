import { describe, it, expect } from 'vitest'
import { useConfirm } from './useConfirm'

describe('useConfirm', () => {
  it('resolves true when respond(true) is called', async () => {
    const { confirm, respond, state } = useConfirm()
    const p = confirm({ message: 'Delete?' })
    expect(state.open).toBe(true)
    respond(true)
    await expect(p).resolves.toBe(true)
    expect(state.open).toBe(false)
  })

  it('resolves false when respond(false) is called', async () => {
    const { confirm, respond } = useConfirm()
    const p = confirm({ message: 'Delete?' })
    respond(false)
    await expect(p).resolves.toBe(false)
  })

  it('cancels a pending confirm when a new one starts', async () => {
    const { confirm, respond } = useConfirm()
    const first = confirm({ message: 'first' })
    const second = confirm({ message: 'second' })
    respond(true)
    await expect(first).resolves.toBe(false)
    await expect(second).resolves.toBe(true)
  })

  it('stores options on the shared state', () => {
    const { confirm, respond, state } = useConfirm()
    confirm({ title: 'T', message: 'M', confirmLabel: 'Yes', tone: 'danger' })
    expect(state.options.title).toBe('T')
    expect(state.options.message).toBe('M')
    expect(state.options.confirmLabel).toBe('Yes')
    expect(state.options.tone).toBe('danger')
    respond(false)
  })
})
