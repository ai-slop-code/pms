import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiIconButton from './UiIconButton.vue'

describe('UiIconButton', () => {
  it('requires and applies aria-label', () => {
    const w = mount(UiIconButton, {
      props: { label: 'Close dialog' },
      slots: { default: '<svg/>' },
    })
    const btn = w.get('button')
    expect(btn.attributes('aria-label')).toBe('Close dialog')
  })

  it('defaults to ghost variant and md size', () => {
    const w = mount(UiIconButton, { props: { label: 'x' } })
    const btn = w.get('button')
    expect(btn.classes()).toContain('ui-iconbtn--ghost')
    expect(btn.classes()).toContain('ui-iconbtn--md')
  })

  it('supports secondary variant and sm size', () => {
    const w = mount(UiIconButton, {
      props: { label: 'x', variant: 'secondary', size: 'sm' },
    })
    const btn = w.get('button')
    expect(btn.classes()).toContain('ui-iconbtn--secondary')
    expect(btn.classes()).toContain('ui-iconbtn--sm')
  })

  it('reflects disabled prop on the native button', () => {
    const w = mount(UiIconButton, { props: { label: 'x', disabled: true } })
    expect(w.get('button').attributes('disabled')).toBeDefined()
  })

  it('emits click', async () => {
    const w = mount(UiIconButton, { props: { label: 'x' } })
    await w.get('button').trigger('click')
    expect(w.emitted('click')).toBeTruthy()
  })
})
