import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiToolbar from './UiToolbar.vue'

describe('UiToolbar', () => {
  it('renders default slot in main region', () => {
    const w = mount(UiToolbar, { slots: { default: '<span data-test="a">A</span>' } })
    expect(w.find('.ui-toolbar__main [data-test="a"]').exists()).toBe(true)
  })

  it('omits trailing region when trailing slot is absent', () => {
    const w = mount(UiToolbar, { slots: { default: 'x' } })
    expect(w.find('.ui-toolbar__trailing').exists()).toBe(false)
  })

  it('renders trailing slot when provided', () => {
    const w = mount(UiToolbar, {
      slots: { default: 'x', trailing: '<button data-test="t">T</button>' },
    })
    expect(w.find('.ui-toolbar__trailing [data-test="t"]').exists()).toBe(true)
  })

  it('applies sticky modifier class when sticky=true', () => {
    const w = mount(UiToolbar, { props: { sticky: true }, slots: { default: 'x' } })
    expect(w.get('.ui-toolbar').classes()).toContain('ui-toolbar--sticky')
  })
})
