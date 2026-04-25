import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick, defineComponent, h, ref } from 'vue'
import UiDialog from './UiDialog.vue'

describe('UiDialog', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
  })
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('teleports into body and renders title when open', async () => {
    mount(UiDialog, {
      props: { open: true, title: 'Confirm action' },
      slots: { default: '<button id="ok">OK</button>' },
      attachTo: document.body,
    })
    await flushPromises()
    const dialog = document.querySelector('[role="dialog"]')
    expect(dialog).not.toBeNull()
    expect(dialog?.getAttribute('aria-modal')).toBe('true')
    expect(document.querySelector('.ui-dialog__title')?.textContent).toContain('Confirm action')
  })

  it('emits update:open=false on Escape key', async () => {
    const wrapper = mount(UiDialog, {
      props: { open: true, title: 'X' },
      attachTo: document.body,
    })
    await flushPromises()
    const dialog = document.querySelector('[role="dialog"]') as HTMLElement
    const evt = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    dialog.dispatchEvent(evt)
    await nextTick()
    const events = wrapper.emitted('update:open')
    expect(events).toBeTruthy()
    expect(events?.[0]).toEqual([false])
  })

  it('does NOT emit update:open on Escape when persistent', async () => {
    const wrapper = mount(UiDialog, {
      props: { open: true, title: 'Locked', persistent: true },
      attachTo: document.body,
    })
    await flushPromises()
    const dialog = document.querySelector('[role="dialog"]') as HTMLElement
    const evt = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    dialog.dispatchEvent(evt)
    await nextTick()
    expect(wrapper.emitted('update:open')).toBeFalsy()
  })

  it('emits update:open=false when backdrop is clicked (non-persistent)', async () => {
    const wrapper = mount(UiDialog, {
      props: { open: true, title: 'Dismissable' },
      attachTo: document.body,
    })
    await flushPromises()
    const backdrop = document.querySelector('.ui-dialog__backdrop') as HTMLElement
    backdrop.click()
    await nextTick()
    expect(wrapper.emitted('update:open')?.[0]).toEqual([false])
  })

  it('does NOT close on backdrop click when persistent', async () => {
    const wrapper = mount(UiDialog, {
      props: { open: true, title: 'Locked', persistent: true },
      attachTo: document.body,
    })
    await flushPromises()
    const backdrop = document.querySelector('.ui-dialog__backdrop') as HTMLElement
    backdrop.click()
    await nextTick()
    expect(wrapper.emitted('update:open')).toBeFalsy()
  })

  it('focuses first focusable element on open', async () => {
    const wrapper = mount(UiDialog, {
      props: { open: false, title: 'Focus', persistent: true },
      slots: { default: '<input id="first" /><input id="second" />' },
      attachTo: document.body,
    })
    await wrapper.setProps({ open: true })
    await flushPromises()
    const first = document.getElementById('first') as HTMLInputElement
    expect(document.activeElement).toBe(first)
  })

  it('traps Tab focus: Shift+Tab on first wraps to last', async () => {
    mount(UiDialog, {
      props: { open: true, title: 'Trap', persistent: true },
      slots: { default: '<button id="a">A</button><button id="b">B</button>' },
      attachTo: document.body,
    })
    await flushPromises()
    const a = document.getElementById('a') as HTMLButtonElement
    const b = document.getElementById('b') as HTMLButtonElement
    a.focus()
    expect(document.activeElement).toBe(a)
    const dialog = document.querySelector('[role="dialog"]') as HTMLElement
    const evt = new KeyboardEvent('keydown', {
      key: 'Tab',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })
    dialog.dispatchEvent(evt)
    await nextTick()
    expect(document.activeElement).toBe(b)
  })

  it('restores focus to previously active element after closing', async () => {
    // Parent host that toggles dialog open
    const Host = defineComponent({
      setup() {
        const open = ref(false)
        return { open }
      },
      render() {
        return h('div', [
          h('button', {
            id: 'opener',
            onClick: () => (this.open = true),
          }, 'Open'),
          h(UiDialog, {
            open: this.open,
            title: 'Restore',
            'onUpdate:open': (v: boolean) => (this.open = v),
          }),
        ])
      },
    })
    mount(Host, { attachTo: document.body })
    const opener = document.getElementById('opener') as HTMLButtonElement
    opener.focus()
    expect(document.activeElement).toBe(opener)
    opener.click()
    await flushPromises()
    expect(document.activeElement).not.toBe(opener)
    // Close via Escape
    const dialog = document.querySelector('[role="dialog"]') as HTMLElement
    dialog.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    await flushPromises()
    expect(document.activeElement).toBe(opener)
  })
})
