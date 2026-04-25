import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiPageHeader from './UiPageHeader.vue'

describe('UiPageHeader', () => {
  it('renders title inside a single <h1>', () => {
    const w = mount(UiPageHeader, { props: { title: 'Dashboard' } })
    const h1s = w.findAll('h1')
    expect(h1s.length).toBe(1)
    expect(h1s[0]?.text()).toBe('Dashboard')
  })

  it('renders lede when provided and omits it otherwise', () => {
    const w = mount(UiPageHeader, { props: { title: 'T', lede: 'Summary line' } })
    expect(w.find('.ui-page-header__lede').text()).toBe('Summary line')

    const w2 = mount(UiPageHeader, { props: { title: 'T' } })
    expect(w2.find('.ui-page-header__lede').exists()).toBe(false)
  })

  it('renders actions and meta slots', () => {
    const w = mount(UiPageHeader, {
      props: { title: 'T' },
      slots: {
        actions: '<button data-test="a">A</button>',
        meta: '<span data-test="m">M</span>',
      },
    })
    expect(w.find('[data-test="a"]').exists()).toBe(true)
    expect(w.find('[data-test="m"]').exists()).toBe(true)
  })
})
