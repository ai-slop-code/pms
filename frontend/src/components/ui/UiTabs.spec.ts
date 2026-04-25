import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiTabs from './UiTabs.vue'

const TABS = [
  { id: 'one', label: 'One' },
  { id: 'two', label: 'Two' },
  { id: 'three', label: 'Three', disabled: true },
]

describe('UiTabs', () => {
  it('renders tabs with role=tab', () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'one', tabs: TABS, ariaLabel: 'demo' },
    })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(3)
    expect(tabs[0]?.attributes('aria-selected')).toBe('true')
    expect(tabs[1]?.attributes('aria-selected')).toBe('false')
    expect(tabs[2]?.attributes('disabled')).toBeDefined()
  })

  it('exposes aria-label on tablist', () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'one', tabs: TABS, ariaLabel: 'demo tabs' },
    })
    const tablist = wrapper.find('[role="tablist"]')
    expect(tablist.attributes('aria-label')).toBe('demo tabs')
  })

  it('emits update:modelValue when a tab is clicked', async () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'one', tabs: TABS },
    })
    await wrapper.findAll('[role="tab"]')[1]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['two'])
  })

  it('arrow keys move selection to enabled tabs', async () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'one', tabs: TABS },
    })
    await wrapper.findAll('[role="tab"]')[0]?.trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['two'])
  })

  it('does not emit when clicking the active tab', async () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'one', tabs: TABS },
    })
    await wrapper.findAll('[role="tab"]')[0]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('active tab gets tabindex=0 and others -1', () => {
    const wrapper = mount(UiTabs, {
      props: { modelValue: 'two', tabs: TABS },
    })
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs[0]?.attributes('tabindex')).toBe('-1')
    expect(tabs[1]?.attributes('tabindex')).toBe('0')
  })
})
