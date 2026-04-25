import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { useDocumentTitle } from './useDocumentTitle'

const BASE = 'PMS'

describe('useDocumentTitle', () => {
  beforeEach(() => {
    document.title = BASE
  })
  afterEach(() => {
    document.title = BASE
  })

  it('prepends the active title segment to the base brand', async () => {
    const Host = defineComponent({
      setup() {
        useDocumentTitle('Dashboard')
        return () => h('div')
      },
    })
    mount(Host)
    await nextTick()
    expect(document.title).toBe(`Dashboard · ${BASE}`)
  })

  it('reacts to a ref / getter source', async () => {
    const title = ref<string | null>('A')
    const Host = defineComponent({
      setup() {
        useDocumentTitle(() => title.value)
        return () => h('div')
      },
    })
    mount(Host)
    await nextTick()
    expect(document.title).toBe(`A · ${BASE}`)

    title.value = 'B'
    await nextTick()
    expect(document.title).toBe(`B · ${BASE}`)

    title.value = null
    await nextTick()
    expect(document.title).toBe(BASE)
  })

  it('restores the base title on unmount', async () => {
    const Host = defineComponent({
      setup() {
        useDocumentTitle('Messages')
        return () => h('div')
      },
    })
    const wrapper = mount(Host)
    await nextTick()
    expect(document.title).toBe(`Messages · ${BASE}`)

    wrapper.unmount()
    expect(document.title).toBe(BASE)
  })
})
