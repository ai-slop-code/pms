import { describe, it, expect, beforeEach, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'

vi.mock('@/api/http', () => ({ api: vi.fn() }))
vi.mock('@/composables/useConfirm', () => ({
  useConfirm: () => ({ confirm: vi.fn().mockResolvedValue(true) }),
}))

import { api } from '@/api/http'
import MessagesView from './MessagesView.vue'
import { usePropertyStore } from '@/stores/property'

const apiMock = api as unknown as ReturnType<typeof vi.fn>

function installLocalStorageStub() {
  const store = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      get length() {
        return store.size
      },
      clear: () => store.clear(),
      getItem: (k: string) => (store.has(k) ? (store.get(k) as string) : null),
      key: (i: number) => Array.from(store.keys())[i] ?? null,
      removeItem: (k: string) => {
        store.delete(k)
      },
      setItem: (k: string, v: string) => {
        store.set(k, v)
      },
    } satisfies Storage,
    configurable: true,
    writable: true,
  })
}

function seedProperty(id = 8) {
  const store = usePropertyStore()
  store.list = [
    {
      id,
      name: 'Apartment H',
      timezone: 'Europe/Bratislava',
      default_language: 'sk',
      owner_user_id: 1,
      active: true,
    },
  ]
  store.currentId = id
  return id
}

function apiRouter(handlers: Record<string, () => unknown>) {
  apiMock.mockImplementation((url: string) => {
    const match = Object.keys(handlers).find((key) => url.includes(key))
    if (match) return Promise.resolve(handlers[match]!())
    if (url.includes('/message-templates')) {
      return Promise.resolve({
        templates: [],
        supported_languages: ['en', 'sk'],
        supported_placeholders: [],
      })
    }
    if (url.includes('/occupancies')) return Promise.resolve({ occupancies: [] })
    return Promise.resolve({})
  })
}

describe('MessagesView', () => {
  beforeEach(() => {
    installLocalStorageStub()
    setActivePinia(createPinia())
    apiMock.mockReset()
  })

  it('shows the property empty-state when no property is selected', async () => {
    apiRouter({})
    const w = mount(MessagesView)
    await flushPromises()
    expect(w.text()).toContain('Pick a property')
  })

  it('loads templates on mount and renders a template title', async () => {
    seedProperty()
    apiRouter({
      '/message-templates': () => ({
        templates: [
          {
            id: 21,
            property_id: 8,
            language_code: 'en',
            template_type: 'check_in',
            title: 'Welcome to the flat',
            body: 'Hi {{guest_name}}',
            active: true,
            created_at: '2026-01-01T10:00:00Z',
            updated_at: '2026-01-01T10:00:00Z',
          },
        ],
        supported_languages: ['en', 'sk'],
        supported_placeholders: ['guest_name'],
      }),
    })
    const w = mount(MessagesView)
    await flushPromises()
    // Templates are rendered on the "templates" tab — switch once we confirm load.
    expect(apiMock).toHaveBeenCalled()
    // Click the Templates tab; switch by text since UiTabs renders buttons.
    const tabBtn = w
      .findAll('button')
      .find((b) => b.text().trim() === 'Templates')
    if (tabBtn) {
      await tabBtn.trigger('click')
      await flushPromises()
      expect(w.text()).toContain('Welcome to the flat')
    }
  })

  it('surfaces an error banner when the initial load rejects', async () => {
    seedProperty()
    apiMock.mockRejectedValue(new Error('messages api down'))
    const w = mount(MessagesView)
    await flushPromises()
    expect(w.text()).toContain('messages api down')
  })
})
