import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiInput from './UiInput.vue'

describe('UiInput', () => {
  it('renders label and forwards for/id', () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', label: 'Email', id: 'email-field' },
    })
    const label = wrapper.find('label')
    const input = wrapper.find('input')
    expect(label.text()).toContain('Email')
    expect(label.attributes('for')).toBe('email-field')
    expect(input.attributes('id')).toBe('email-field')
  })

  it('emits update:modelValue on input', async () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', label: 'L' },
    })
    await wrapper.find('input').setValue('hello')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['hello'])
  })

  it('forwards min/max to the input (date range bounds)', () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', label: 'D', type: 'date', min: '2026-07-09', max: '2026-07-11' },
    })
    const input = wrapper.find('input')
    expect(input.attributes('min')).toBe('2026-07-09')
    expect(input.attributes('max')).toBe('2026-07-11')
  })

  it('forwards step to the native input and preserves numeric validity', () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', type: 'number', min: 0, step: '0.01' },
    })
    const input = wrapper.find('input').element as HTMLInputElement

    expect(input.getAttribute('step')).toBe('0.01')

    input.value = '123.99'
    expect(input.validity.stepMismatch).toBe(false)
    input.value = '123.999'
    expect(input.validity.stepMismatch).toBe(true)
    input.value = '-0.01'
    expect(input.validity.rangeUnderflow).toBe(true)
  })

  it('preserves explicit integer steps and omits step when not supplied', () => {
    const integerStep = mount(UiInput, {
      props: { modelValue: '', type: 'number', min: 0, step: 1 },
    }).find('input').element as HTMLInputElement
    integerStep.value = '1.5'

    expect(integerStep.getAttribute('step')).toBe('1')
    expect(integerStep.validity.stepMismatch).toBe(true)

    const defaultStep = mount(UiInput, { props: { modelValue: '', type: 'number' } }).find('input')
    expect(defaultStep.attributes('step')).toBeUndefined()
  })

  it('renders help text with aria-describedby linkage', () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', label: 'L', help: 'Some help' },
    })
    const input = wrapper.find('input')
    const described = input.attributes('aria-describedby')
    expect(described).toBeTruthy()
    const helpEl = wrapper.find(`#${described}`)
    expect(helpEl.text()).toBe('Some help')
  })

  it('renders error text, sets aria-invalid and replaces help', () => {
    const wrapper = mount(UiInput, {
      props: {
        modelValue: '',
        label: 'L',
        help: 'Some help',
        error: 'Required',
      },
    })
    const input = wrapper.find('input')
    expect(input.attributes('aria-invalid')).toBe('true')
    expect(wrapper.text()).toContain('Required')
    expect(wrapper.text()).not.toContain('Some help')
  })

  it('forwards type attribute', () => {
    const wrapper = mount(UiInput, {
      props: { modelValue: '', label: 'L', type: 'number' },
    })
    expect(wrapper.find('input').attributes('type')).toBe('number')
  })
})
