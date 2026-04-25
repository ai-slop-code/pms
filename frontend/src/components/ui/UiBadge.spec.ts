import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiBadge from './UiBadge.vue'

describe('UiBadge', () => {
  it('renders label prop', () => {
    const wrapper = mount(UiBadge, { props: { label: 'Active' } })
    expect(wrapper.text()).toContain('Active')
  })

  it('renders slot content over label', () => {
    const wrapper = mount(UiBadge, {
      props: { label: 'ignored' },
      slots: { default: 'Slot wins' },
    })
    expect(wrapper.text()).toContain('Slot wins')
  })

  it('applies tone and size classes', () => {
    const wrapper = mount(UiBadge, {
      props: { tone: 'success', size: 'sm', label: 'OK' },
    })
    expect(wrapper.classes()).toContain('ui-badge--success')
    expect(wrapper.classes()).toContain('ui-badge--sm')
  })

  it('renders a decorative dot when dot=true', () => {
    const wrapper = mount(UiBadge, { props: { label: 'Live', dot: true } })
    const dot = wrapper.find('.ui-badge__dot')
    expect(dot.exists()).toBe(true)
    expect(dot.attributes('aria-hidden')).toBe('true')
  })

  it('defaults to neutral/md', () => {
    const wrapper = mount(UiBadge, { props: { label: 'n/a' } })
    expect(wrapper.classes()).toContain('ui-badge--neutral')
    expect(wrapper.classes()).toContain('ui-badge--md')
  })
})
