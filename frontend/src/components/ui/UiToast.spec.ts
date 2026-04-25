import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiToast from './UiToast.vue'

describe('UiToast', () => {
  it('uses role=status and aria-live=polite by default', () => {
    const w = mount(UiToast, { props: { title: 'Hi', message: 'There' } })
    const el = w.get('.ui-toast')
    expect(el.attributes('role')).toBe('status')
    expect(el.attributes('aria-live')).toBe('polite')
  })

  it('uses aria-live=assertive for danger tone', () => {
    const w = mount(UiToast, { props: { tone: 'danger', title: 'Oops' } })
    expect(w.get('.ui-toast').attributes('aria-live')).toBe('assertive')
  })

  it('applies tone modifier class', () => {
    const w = mount(UiToast, { props: { tone: 'success', title: 'Ok' } })
    expect(w.get('.ui-toast').classes()).toContain('ui-toast--success')
  })

  it('emits dismiss when the close button is clicked', async () => {
    const w = mount(UiToast, { props: { title: 'x' } })
    await w.get('.ui-toast__close').trigger('click')
    expect(w.emitted('dismiss')).toBeTruthy()
  })
})
