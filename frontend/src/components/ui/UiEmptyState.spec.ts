import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiEmptyState from './UiEmptyState.vue'

describe('UiEmptyState', () => {
  it('renders title and description', () => {
    const w = mount(UiEmptyState, {
      props: { title: 'No items', description: 'Add one' },
    })
    expect(w.find('.ui-empty__title').text()).toBe('No items')
    expect(w.find('.ui-empty__description').text()).toBe('Add one')
  })

  it('omits description when missing', () => {
    const w = mount(UiEmptyState, { props: { title: 'Empty' } })
    expect(w.find('.ui-empty__description').exists()).toBe(false)
  })

  it('renders icon slot and hides it from assistive tech', () => {
    const w = mount(UiEmptyState, {
      props: { title: 'x' },
      slots: { icon: '<svg data-test="ic"/>' },
    })
    const ic = w.find('.ui-empty__icon')
    expect(ic.exists()).toBe(true)
    expect(ic.attributes('aria-hidden')).toBe('true')
  })

  it('renders actions slot when provided', () => {
    const w = mount(UiEmptyState, {
      props: { title: 'x' },
      slots: { actions: '<button>Go</button>' },
    })
    expect(w.find('.ui-empty__actions').text()).toBe('Go')
  })

  it('renders a named illustration when the prop is set', () => {
    const w = mount(UiEmptyState, {
      props: { title: 'x', illustration: 'inbox' },
    })
    const slot = w.find('.ui-empty__illustration')
    expect(slot.exists()).toBe(true)
    expect(slot.attributes('aria-hidden')).toBe('true')
    expect(slot.find('svg').exists()).toBe(true)
  })

  it('prefers the icon slot over the illustration prop when both are provided', () => {
    const w = mount(UiEmptyState, {
      props: { title: 'x', illustration: 'inbox' },
      slots: { icon: '<i data-test="icon"/>' },
    })
    expect(w.find('.ui-empty__icon').exists()).toBe(true)
    expect(w.find('.ui-empty__illustration').exists()).toBe(false)
  })
})
