import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiSelect from './UiSelect.vue'

describe('UiSelect', () => {
  it('links label to select via for/id', () => {
    const w = mount(UiSelect, {
      props: { label: 'Role', modelValue: 'a' },
      slots: { default: '<option value="a">A</option><option value="b">B</option>' },
    })
    const select = w.get('select')
    const label = w.get('label')
    expect(label.attributes('for')).toBe(select.attributes('id'))
  })

  it('emits update:modelValue on change', async () => {
    const w = mount(UiSelect, {
      props: { modelValue: 'a' },
      slots: { default: '<option value="a">A</option><option value="b">B</option>' },
    })
    const select = w.get('select').element as HTMLSelectElement
    select.value = 'b'
    await w.get('select').trigger('change')
    expect(w.emitted('update:modelValue')?.[0]).toEqual(['b'])
  })

  it('exposes aria-invalid and aria-describedby when error is set', () => {
    const w = mount(UiSelect, {
      props: { modelValue: '', label: 'X', error: 'Required' },
      slots: { default: '<option value="">-</option>' },
    })
    const select = w.get('select')
    expect(select.attributes('aria-invalid')).toBe('true')
    expect(select.attributes('aria-describedby')).toMatch(/-error$/)
  })

  it('passes disabled and required flags to the native select', () => {
    const w = mount(UiSelect, {
      props: { modelValue: '', disabled: true, required: true },
      slots: { default: '<option value="">-</option>' },
    })
    const select = w.get('select')
    expect(select.attributes('disabled')).toBeDefined()
    expect(select.attributes('required')).toBeDefined()
  })
})
