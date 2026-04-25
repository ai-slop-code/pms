import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiButton from './UiButton.vue'

describe('UiButton', () => {
  it('renders the default slot', () => {
    const wrapper = mount(UiButton, { slots: { default: 'Save' } })
    expect(wrapper.text()).toContain('Save')
  })

  it('applies variant and size classes', () => {
    const wrapper = mount(UiButton, {
      props: { variant: 'primary', size: 'lg' },
      slots: { default: 'Go' },
    })
    expect(wrapper.classes()).toContain('ui-btn--primary')
    expect(wrapper.classes()).toContain('ui-btn--lg')
  })

  it('defaults to secondary/md', () => {
    const wrapper = mount(UiButton, { slots: { default: 'Go' } })
    expect(wrapper.classes()).toContain('ui-btn--secondary')
    expect(wrapper.classes()).toContain('ui-btn--md')
  })

  it('is aria-busy and disabled when loading', () => {
    const wrapper = mount(UiButton, {
      props: { loading: true },
      slots: { default: 'Loading' },
    })
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.classes()).toContain('ui-btn--loading')
  })

  it('forwards ariaLabel prop as aria-label', () => {
    const wrapper = mount(UiButton, {
      props: { ariaLabel: 'Close drawer' },
    })
    expect(wrapper.attributes('aria-label')).toBe('Close drawer')
  })

  it('respects disabled prop', () => {
    const wrapper = mount(UiButton, {
      props: { disabled: true },
      slots: { default: 'Go' },
    })
    expect(wrapper.attributes('disabled')).toBeDefined()
  })

  it('renders iconLeft slot when not loading and default slot as label', () => {
    const wrapper = mount(UiButton, {
      slots: {
        default: 'Add',
        iconLeft: '<span class="mock-icon" />',
      },
    })
    expect(wrapper.find('.mock-icon').exists()).toBe(true)
    expect(wrapper.find('.ui-btn__label').text()).toBe('Add')
  })
})
