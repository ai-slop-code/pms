import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiTable from './UiTable.vue'

describe('UiTable', () => {
  it('renders caption when provided', () => {
    const w = mount(UiTable, {
      props: { caption: 'Transactions' },
      slots: { head: '<tr><th>H</th></tr>', default: '<tr><td>x</td></tr>' },
    })
    expect(w.find('caption').text()).toBe('Transactions')
  })

  it('renders empty row when empty=true', () => {
    const w = mount(UiTable, {
      props: { empty: true, emptyText: 'Nothing here' },
      slots: { head: '<tr><th>H</th></tr>' },
    })
    expect(w.find('.ui-table__empty-row td').text()).toBe('Nothing here')
  })

  it('applies sticky and dense modifier classes', () => {
    const w = mount(UiTable, {
      props: { stickyHeader: true, dense: true },
      slots: { head: '<tr><th>H</th></tr>', default: '<tr><td>x</td></tr>' },
    })
    const t = w.get('table')
    expect(t.classes()).toContain('ui-table--sticky')
    expect(t.classes()).toContain('ui-table--dense')
  })

  it('exposes data-stack attribute when stack=true', () => {
    const w = mount(UiTable, {
      props: { stack: true },
      slots: { head: '<tr><th>H</th></tr>', default: '<tr><td>x</td></tr>' },
    })
    const t = w.get('table')
    expect(t.classes()).toContain('ui-table--stack')
    expect(t.attributes('data-stack')).toBe('true')
  })

  it('omits thead when head slot is absent', () => {
    const w = mount(UiTable, { slots: { default: '<tr><td>x</td></tr>' } })
    expect(w.find('thead').exists()).toBe(false)
  })
})
