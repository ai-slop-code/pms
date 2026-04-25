import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiKpiCard from './UiKpiCard.vue'

describe('UiKpiCard', () => {
  it('renders label and value', () => {
    const wrapper = mount(UiKpiCard, {
      props: { label: 'Revenue', value: '€1,200' },
    })
    expect(wrapper.text()).toContain('Revenue')
    expect(wrapper.text()).toContain('€1,200')
  })

  it('adds hero class when hero=true', () => {
    const wrapper = mount(UiKpiCard, {
      props: { label: 'Net', value: '€5,000', hero: true },
    })
    expect(wrapper.classes()).toContain('ui-kpi--hero')
  })

  it('applies tone class', () => {
    const wrapper = mount(UiKpiCard, {
      props: { label: 'Losses', value: '€-100', tone: 'danger' },
    })
    expect(wrapper.classes()).toContain('ui-kpi--tone-danger')
  })

  it('renders trend with up direction as success by default', () => {
    const wrapper = mount(UiKpiCard, {
      props: {
        label: 'Revenue',
        value: '€1,200',
        trend: { direction: 'up', label: '+8%' },
      },
    })
    const trend = wrapper.find('.ui-kpi__trend')
    expect(trend.exists()).toBe(true)
    expect(trend.classes()).toContain('ui-kpi__trend--success')
    expect(trend.text()).toContain('+8%')
  })

  it('renders trend with down direction as danger by default', () => {
    const wrapper = mount(UiKpiCard, {
      props: {
        label: 'Losses',
        value: '€-100',
        trend: { direction: 'down', label: '-5%' },
      },
    })
    expect(wrapper.find('.ui-kpi__trend--danger').exists()).toBe(true)
  })

  it('honours explicit trend tone override', () => {
    const wrapper = mount(UiKpiCard, {
      props: {
        label: 'Neutral',
        value: '0',
        trend: { direction: 'up', label: 'meh', tone: 'neutral' },
      },
    })
    expect(wrapper.find('.ui-kpi__trend--neutral').exists()).toBe(true)
  })

  it('renders hint when provided', () => {
    const wrapper = mount(UiKpiCard, {
      props: { label: 'L', value: '1', hint: 'Since last week' },
    })
    expect(wrapper.find('.ui-kpi__hint').text()).toBe('Since last week')
  })
})
