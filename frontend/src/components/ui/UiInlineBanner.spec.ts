import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiInlineBanner from './UiInlineBanner.vue'

describe('UiInlineBanner', () => {
  it('renders title and message', () => {
    const wrapper = mount(UiInlineBanner, {
      props: { title: 'Saved' },
      slots: { default: 'Your changes were saved.' },
    })
    expect(wrapper.find('.ui-banner__title').text()).toBe('Saved')
    expect(wrapper.find('.ui-banner__message').text()).toBe('Your changes were saved.')
  })

  it('applies tone modifier', () => {
    const wrapper = mount(UiInlineBanner, {
      props: { tone: 'success', title: 'ok' },
    })
    expect(wrapper.classes()).toContain('ui-banner--success')
  })

  it('uses aria-live=assertive for danger and polite otherwise', () => {
    const warn = mount(UiInlineBanner, { props: { tone: 'warning', title: 'h' } })
    expect(warn.attributes('aria-live')).toBe('polite')
    const danger = mount(UiInlineBanner, { props: { tone: 'danger', title: 'e' } })
    expect(danger.attributes('aria-live')).toBe('assertive')
  })

  it('hides icon when icon=false', () => {
    const wrapper = mount(UiInlineBanner, {
      props: { tone: 'info', title: 'h', icon: false },
    })
    expect(wrapper.find('.ui-banner__icon').exists()).toBe(false)
  })

  it('renders actions slot', () => {
    const wrapper = mount(UiInlineBanner, {
      props: { tone: 'info', title: 'h' },
      slots: { actions: '<button class="mock-action">Act</button>' },
    })
    expect(wrapper.find('.ui-banner__actions .mock-action').exists()).toBe(true)
  })
})
