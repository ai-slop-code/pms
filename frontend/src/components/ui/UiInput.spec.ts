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
