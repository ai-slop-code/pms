import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiFileInput from './UiFileInput.vue'

function makeFile(name = 'doc.pdf') {
  return new File(['dummy'], name, { type: 'application/pdf' })
}

describe('UiFileInput', () => {
  it('renders label and button, with empty-state copy', () => {
    const wrapper = mount(UiFileInput, {
      props: { modelValue: null, label: 'Attachment' },
    })
    expect(wrapper.find('label').text()).toContain('Attachment')
    expect(wrapper.find('.ui-file__btn').text()).toContain('Choose file')
    expect(wrapper.find('.ui-file__empty').text()).toBe('No file selected')
  })

  it('shows the filename when a file is present', () => {
    const file = makeFile('receipt.pdf')
    const wrapper = mount(UiFileInput, {
      props: { modelValue: file, label: 'Attachment' },
    })
    expect(wrapper.find('.ui-file__name').text()).toBe('receipt.pdf')
    expect(wrapper.find('.ui-file__empty').exists()).toBe(false)
  })

  it('emits null when cleared', async () => {
    const file = makeFile()
    const wrapper = mount(UiFileInput, {
      props: { modelValue: file, label: 'Attachment' },
    })
    await wrapper.find('.ui-file__clear').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([null])
  })

  it('does not render clear button when disabled', () => {
    const file = makeFile()
    const wrapper = mount(UiFileInput, {
      props: { modelValue: file, label: 'Attachment', disabled: true },
    })
    expect(wrapper.find('.ui-file__clear').exists()).toBe(false)
    expect(wrapper.find('.ui-file__btn').attributes('disabled')).toBeDefined()
  })

  it('sets aria-describedby when help or error is present', () => {
    const wrapper = mount(UiFileInput, {
      props: { modelValue: null, label: 'Attachment', help: 'Max 10 MB' },
    })
    const described = wrapper.find('.ui-file__btn').attributes('aria-describedby')
    expect(described).toBeTruthy()
    expect(wrapper.find(`#${described}`).text()).toBe('Max 10 MB')
  })

  it('renders error message and aria-invalid', () => {
    const wrapper = mount(UiFileInput, {
      props: { modelValue: null, label: 'Attachment', error: 'Required' },
    })
    expect(wrapper.text()).toContain('Required')
    expect(wrapper.find('input[type="file"]').attributes('aria-invalid')).toBe('true')
  })
})
