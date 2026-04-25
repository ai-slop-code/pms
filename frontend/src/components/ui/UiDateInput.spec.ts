import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import UiDateInput from './UiDateInput.vue'

describe('UiDateInput', () => {
  it('renders a native date input with the provided label linked to it', () => {
    const w = mount(UiDateInput, { props: { modelValue: '2026-04-23', label: 'From' } })
    const input = w.get('input')
    expect(input.attributes('type')).toBe('date')
    expect((input.element as HTMLInputElement).value).toBe('2026-04-23')
    const label = w.get('label')
    expect(label.attributes('for')).toBe(input.attributes('id'))
  })

  it('emits update:modelValue on input', async () => {
    const w = mount(UiDateInput, { props: { modelValue: '2026-01-01' } })
    const input = w.get('input')
    ;(input.element as HTMLInputElement).value = '2026-02-02'
    await input.trigger('input')
    const events = w.emitted('update:modelValue')
    expect(events).toBeTruthy()
    expect(events?.[0]).toEqual(['2026-02-02'])
  })

  it('passes error and aria-invalid through to the underlying input', () => {
    const w = mount(UiDateInput, {
      props: { modelValue: '', label: 'Day', error: 'Required' },
    })
    const input = w.get('input')
    expect(input.attributes('aria-invalid')).toBe('true')
    expect(input.attributes('aria-describedby')).toMatch(/-error$/)
  })

  it('accepts null modelValue and treats it as empty', () => {
    const w = mount(UiDateInput, { props: { modelValue: null } })
    expect((w.get('input').element as HTMLInputElement).value).toBe('')
  })
})
