import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiSection from './UiSection.vue'

describe('UiSection', () => {
  it('renders title as h2 and description when provided', () => {
    const w = mount(UiSection, {
      props: { title: 'Finance', description: 'Ledger' },
      slots: { default: '<div>body</div>' },
    })
    expect(w.find('h2').text()).toBe('Finance')
    expect(w.find('.ui-section__description').text()).toBe('Ledger')
  })

  it('omits header entirely when no title/description/actions', () => {
    const w = mount(UiSection, { slots: { default: 'body' } })
    expect(w.find('.ui-section__header').exists()).toBe(false)
  })

  it('renders actions slot in header', () => {
    const w = mount(UiSection, {
      props: { title: 'T' },
      slots: {
        default: 'x',
        actions: '<button data-test="act">Go</button>',
      },
    })
    expect(w.find('.ui-section__actions [data-test="act"]').exists()).toBe(true)
  })
})
