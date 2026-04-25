import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiCard from './UiCard.vue'

describe('UiCard', () => {
  it('renders default slot inside .ui-card__body', () => {
    const w = mount(UiCard, { slots: { default: '<p>Hello</p>' } })
    expect(w.find('.ui-card__body').html()).toContain('<p>Hello</p>')
  })

  it('omits header and footer when no slots provided', () => {
    const w = mount(UiCard, { slots: { default: 'x' } })
    expect(w.find('.ui-card__header').exists()).toBe(false)
    expect(w.find('.ui-card__footer').exists()).toBe(false)
  })

  it('renders header and footer slots when provided', () => {
    const w = mount(UiCard, {
      slots: { default: 'body', header: 'HEAD', footer: 'FOOT' },
    })
    expect(w.find('.ui-card__header').text()).toBe('HEAD')
    expect(w.find('.ui-card__footer').text()).toBe('FOOT')
  })

  it('applies tone and padding modifier classes', () => {
    const w = mount(UiCard, {
      props: { tone: 'sunken', padding: 'tight' },
      slots: { default: 'x' },
    })
    const el = w.find('.ui-card')
    expect(el.classes()).toContain('ui-card--tone-sunken')
    expect(el.classes()).toContain('ui-card--pad-tight')
  })
})
