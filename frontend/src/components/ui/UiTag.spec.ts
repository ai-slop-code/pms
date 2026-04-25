import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiTag from './UiTag.vue'

describe('UiTag', () => {
  it('renders label as default slot content', () => {
    const w = mount(UiTag, { props: { label: 'Draft' } })
    expect(w.text()).toContain('Draft')
  })

  it('allows a custom default slot to override the label text', () => {
    const w = mount(UiTag, {
      props: { label: 'fallback' },
      slots: { default: 'Override' },
    })
    expect(w.text()).toContain('Override')
    expect(w.text()).not.toContain('fallback')
  })

  it('applies tone modifier class', () => {
    const w = mount(UiTag, { props: { label: 'x', tone: 'success' } })
    expect(w.get('.ui-tag').classes()).toContain('ui-tag--success')
  })

  it('renders remove button with an aria-label when removable=true and emits remove', async () => {
    const w = mount(UiTag, { props: { label: 'Foo', removable: true } })
    const btn = w.get('.ui-tag__remove')
    expect(btn.attributes('aria-label')).toBe('Remove Foo')
    await btn.trigger('click')
    expect(w.emitted('remove')).toBeTruthy()
  })

  it('does not render the remove button by default', () => {
    const w = mount(UiTag, { props: { label: 'x' } })
    expect(w.find('.ui-tag__remove').exists()).toBe(false)
  })
})
